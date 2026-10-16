package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/pkg/models"
)

type QueueItem struct {
	TorrentID, TorrentName string
	Target                 models.DownloadTarget
}

type BulkPlan struct{ Items []QueueItem }

type QueueDecision struct {
	QueueID   QueueID
	ItemIndex int
	Overwrite bool
}

type QueueItemStatus string

const (
	QueueItemPending           QueueItemStatus = "pending"
	QueueItemResolving         QueueItemStatus = "resolving"
	QueueItemAwaitingOverwrite QueueItemStatus = "awaiting-overwrite"
	QueueItemActive            QueueItemStatus = "active"
	QueueItemTerminal          QueueItemStatus = "terminal"
)

type QueueItemSnapshot struct {
	Item    QueueItem
	Status  QueueItemStatus
	Outcome *ItemOutcome
}

type QueueSnapshot struct {
	ID                         QueueID
	Items                      []QueueItemSnapshot
	CurrentIndex               int
	Download                   *DownloadSnapshot
	AwaitingOverwrite          bool
	ExistingPath               string
	ExistingBytes, RemoteBytes int64
	Finished                   bool
}

type QueueDependencies struct {
	Runtime   *Runtime
	Downloads *DownloadCoordinator
	Prepare   func(context.Context, models.DownloadTarget) (PreparedDownload, error)
	Wait      WaitFunc
}

type BulkQueue struct {
	mu    sync.Mutex
	deps  QueueDependencies
	state *queueState
}

type queueState struct {
	snapshot        QueueSnapshot
	ctx             context.Context
	cancel          context.CancelFunc
	decision        chan bool
	decisionPending bool
}

func NewBulkQueue(deps QueueDependencies) *BulkQueue {
	if deps.Wait == nil {
		deps.Wait = waitBatch
	}
	return &BulkQueue{deps: deps}
}

func (q *BulkQueue) Start(ctx context.Context, plan BulkPlan) (QueueID, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	lease, err := q.deps.Downloads.AcquireQueue(ctx)
	if err != nil {
		return 0, err
	}
	workerCtx, cancel := context.WithCancel(q.deps.Runtime.Context())
	s := &queueState{ctx: workerCtx, cancel: cancel, decision: make(chan bool, 1), snapshot: QueueSnapshot{ID: lease.ID, CurrentIndex: -1}}
	for _, item := range plan.Items {
		s.snapshot.Items = append(s.snapshot.Items, QueueItemSnapshot{Item: item, Status: QueueItemPending})
	}
	// The caller bounds admission only; admitted work belongs to the runtime.
	err = q.deps.Runtime.Spawn(workerCtx, func(ctx context.Context) { q.run(ctx, s, lease) })
	if err != nil {
		cancel()
		return 0, errors.Join(err, q.deps.Downloads.ReleaseQueue(lease))
	}
	q.state = s
	return lease.ID, nil
}

func (q *BulkQueue) Snapshot(id QueueID) (QueueSnapshot, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.state == nil || q.state.snapshot.ID != id {
		return QueueSnapshot{}, ErrStaleQueue
	}
	s := q.state.snapshot
	s.Items = slices.Clone(s.Items)
	for i := range s.Items {
		if s.Items[i].Outcome != nil {
			s.Items[i].Outcome = new(*s.Items[i].Outcome)
		}
	}
	if s.Download != nil {
		s.Download = new(*s.Download)
	}
	return s, nil
}

func (q *BulkQueue) Decide(ctx context.Context, decision QueueDecision) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	s := q.state
	if s == nil || s.snapshot.ID != decision.QueueID || s.snapshot.CurrentIndex != decision.ItemIndex || !s.decisionPending || s.ctx.Err() != nil {
		return ErrStaleQueue
	}
	s.decisionPending = false
	s.decision <- decision.Overwrite
	return nil
}

// Cancel requests cancellation; Finished is published only after owned cleanup.
func (q *BulkQueue) Cancel(id QueueID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.state == nil || q.state.snapshot.ID != id {
		return ErrStaleQueue
	}
	q.state.cancel()
	return nil
}

func (q *BulkQueue) run(ctx context.Context, s *queueState, lease QueueLease) {
	defer s.cancel()
	var pending *ItemOutcome
	for i, item := range s.snapshot.Items {
		if ctx.Err() != nil {
			break
		}
		q.mu.Lock()
		if pending != nil {
			previous := &s.snapshot.Items[s.snapshot.CurrentIndex]
			previous.Status, previous.Outcome = QueueItemTerminal, pending
		}
		s.snapshot.CurrentIndex = i
		s.snapshot.Download = nil
		s.snapshot.Items[i].Status = QueueItemResolving
		s.snapshot.ExistingPath = ""
		s.snapshot.ExistingBytes, s.snapshot.RemoteBytes = 0, 0
		q.mu.Unlock()
		outcome, abort := q.runItem(ctx, s, lease, i, item.Item)
		pending = new(outcome)
		if abort {
			break
		}
	}
	// Release cannot make a still-active or unconfirmed transfer available. The
	// item worker has already awaited cancellation before reaching this point.
	q.mu.Lock()
	defer q.mu.Unlock()
	releaseErr := q.deps.Downloads.ReleaseQueue(lease)
	if pending != nil {
		// Publish the final item only after cleanup, so a release failure is
		// retained without changing an already published terminal outcome.
		if releaseErr != nil {
			pending.Err = errors.Join(pending.Err, releaseErr)
			if pending.Kind == OutcomeSuccess || pending.Kind == OutcomeSkipped {
				pending.Kind = OutcomeFailed
			}
			pending.RemoteOutcomeUnknown = pending.RemoteOutcomeUnknown || errors.Is(releaseErr, ErrTransferStopUnconfirmed)
		}
		last := &s.snapshot.Items[s.snapshot.CurrentIndex]
		last.Status, last.Outcome = QueueItemTerminal, pending
	}
	for i := range s.snapshot.Items {
		if s.snapshot.Items[i].Outcome == nil {
			s.snapshot.Items[i].Status = QueueItemTerminal
			s.snapshot.Items[i].Outcome = new(queueOutcome(s.snapshot.Items[i].Item, OutcomeCanceled, context.Canceled))
		}
	}
	s.snapshot.AwaitingOverwrite = false
	s.decisionPending = false
	s.snapshot.Finished = true
}

func queueOutcome(item QueueItem, kind OutcomeKind, err error) ItemOutcome {
	return ItemOutcome{TorrentID: item.TorrentID, TargetIndex: item.Target.Index, Source: item.Target.Link, Kind: kind, Err: err}
}

func (q *BulkQueue) runItem(ctx context.Context, s *queueState, lease QueueLease, index int, item QueueItem) (ItemOutcome, bool) {
	p, err := q.deps.Prepare(ctx, item.Target)
	if ctx.Err() != nil {
		return queueOutcome(item, OutcomeCanceled, ctx.Err()), true
	}
	if err != nil {
		return queueOutcome(item, OutcomeFailed, err), false
	}
	if p.Exists {
		q.mu.Lock()
		s.snapshot.Items[index].Status = QueueItemAwaitingOverwrite
		s.snapshot.AwaitingOverwrite, s.decisionPending = true, true
		s.snapshot.ExistingPath = p.Path
		s.snapshot.ExistingBytes, s.snapshot.RemoteBytes = p.ExistingBytes, p.RemoteBytes
		q.mu.Unlock()
		var overwrite bool
		select {
		case overwrite = <-s.decision:
		case <-ctx.Done():
		}
		q.mu.Lock()
		s.snapshot.AwaitingOverwrite, s.decisionPending = false, false
		q.mu.Unlock()
		if ctx.Err() != nil {
			return queueOutcome(item, OutcomeCanceled, ctx.Err()), true
		}
		if !overwrite {
			return queueOutcome(item, OutcomeSkipped, nil), false
		}
	}
	started, err := q.deps.Downloads.StartQueued(ctx, lease, p.URL, p.Filename)
	if errors.Is(err, download.ErrUncertainStart) {
		outcome := queueOutcome(item, OutcomeFailed, err)
		outcome.RemoteOutcomeUnknown = true
		return outcome, true
	}
	if ctx.Err() != nil {
		return q.cancelTransfer(s, lease, started.Snapshot.Ref, item, ctx.Err()), true
	}
	if err != nil {
		outcome := queueOutcome(item, OutcomeFailed, err)
		outcome.RemoteOutcomeUnknown = errors.Is(err, download.ErrUncertainStart)
		return outcome, outcome.RemoteOutcomeUnknown || errors.Is(err, download.ErrClosed) || errors.Is(err, ErrTransferStopUnconfirmed)
	}
	current := started.Snapshot
	for {
		q.mu.Lock()
		s.snapshot.Items[index].Status = QueueItemActive
		s.snapshot.Download = new(current)
		q.mu.Unlock()
		if ctx.Err() != nil {
			return q.cancelTransfer(s, lease, current.Ref, item, ctx.Err()), true
		}
		if current.Download.IsTerminal() {
			outcome := queueOutcome(item, OutcomeSuccess, nil)
			outcome.URL = p.URL
			if !current.Download.IsComplete() {
				outcome.Kind = OutcomeFailed
				outcome.Err = errors.New(current.Download.ErrorMessage)
				if current.Download.Status == models.ManagedDownloadStatusRemoved {
					outcome.Kind, outcome.Err = OutcomeCanceled, context.Canceled
				}
			}
			return outcome, false
		}
		observed, statusErr := q.deps.Downloads.Status(ctx, current.Ref)
		if observed.Ref == current.Ref {
			current = observed
			q.mu.Lock()
			s.snapshot.Download = new(current)
			q.mu.Unlock()
		}
		if ctx.Err() != nil {
			return q.cancelTransfer(s, lease, current.Ref, item, ctx.Err()), true
		}
		if current.Download.IsTerminal() {
			continue
		}
		if errors.Is(statusErr, download.ErrClosed) || errors.Is(statusErr, download.ErrUncertainStart) || errors.Is(statusErr, ErrTransferStopUnconfirmed) || errors.Is(statusErr, ErrStaleDownload) {
			outcome := q.cancelTransfer(s, lease, current.Ref, item, statusErr)
			outcome.Kind = OutcomeFailed
			return outcome, true
		}
		if err := q.deps.Wait(ctx, time.Second); err != nil {
			return q.cancelTransfer(s, lease, current.Ref, item, err), true
		}
	}
}

func (q *BulkQueue) cancelTransfer(s *queueState, lease QueueLease, ref DownloadRef, item QueueItem, cause error) ItemOutcome {
	// Do not reuse the canceled worker context. Preserve an earlier root deadline
	// while giving an ordinary queue cancellation its independent stop budget.
	deadline := time.Now().Add(2 * time.Second)
	if rootDeadline, ok := q.deps.Runtime.Context().Deadline(); ok && rootDeadline.Before(deadline) {
		deadline = rootDeadline
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	err := q.deps.Downloads.CancelQueued(ctx, lease, ref)
	outcome := queueOutcome(item, OutcomeCanceled, errors.Join(cause, err))
	outcome.RemoteOutcomeUnknown = errors.Is(err, ErrTransferStopUnconfirmed) || errors.Is(err, download.ErrUncertainStart)
	if errors.Is(err, download.ErrUncertainStart) {
		outcome.Kind = OutcomeFailed
	}
	if current, ok := q.deps.Downloads.Current(); ok && current.Ref == ref {
		q.mu.Lock()
		s.snapshot.Download = new(current)
		q.mu.Unlock()
	}
	return outcome
}
