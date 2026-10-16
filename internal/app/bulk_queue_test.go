package app

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/pkg/models"
)

// Removing queue-owned advancement, lease checks, or stop confirmation breaks
// these tests: the real runtime/coordinator run against gated backend I/O.
type queueRig struct {
	q        *BulkQueue
	c        *DownloadCoordinator
	r        *Runtime
	b        *coordinatorBackend
	waits    chan chan struct{}
	prepared chan models.DownloadTarget
}

type serialQueueBackend struct {
	download.Backend
	mu               sync.Mutex
	active, parallel bool
	order            []string
}

func (b *serialQueueBackend) StartDownload(ctx context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
	b.mu.Lock()
	b.parallel = b.parallel || b.active
	b.active = true
	b.order = append(b.order, req.URL)
	b.mu.Unlock()
	d, err := b.Backend.StartDownload(ctx, req)
	if err != nil || d.IsTerminal() {
		b.mu.Lock()
		b.active = false
		b.mu.Unlock()
	}
	return d, err
}

func (b *serialQueueBackend) DownloadStatus(ctx context.Context, gid string) (models.ManagedDownload, error) {
	d, err := b.Backend.DownloadStatus(ctx, gid)
	if err == nil && d.IsTerminal() {
		b.mu.Lock()
		b.active = false
		b.mu.Unlock()
	}
	return d, err
}

func newQueueRig(t *testing.T, exists bool) *queueRig {
	t.Helper()
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	r := NewRuntime(t.Context())
	waits := make(chan chan struct{}, 16)
	wait := func(ctx context.Context, _ time.Duration) error {
		gate := make(chan struct{})
		select {
		case waits <- gate:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-gate:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	prepared := make(chan models.DownloadTarget, 16)
	q := NewBulkQueue(QueueDependencies{Runtime: r, Downloads: c, Wait: wait, Prepare: func(ctx context.Context, target models.DownloadTarget) (PreparedDownload, error) {
		prepared <- target
		return PreparedDownload{URL: target.Link, Filename: target.Label, Path: "/downloads/" + target.Label, Exists: exists, ExistingBytes: 5, RemoteBytes: 10}, ctx.Err()
	}})
	t.Cleanup(func() {
		r.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Closing the coordinator also interrupts any backend query left by a
		// failed assertion, so cleanup is independent of the canceled test.
		if err := c.Close(ctx); err != nil {
			t.Error(err)
		}
		if err := r.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
	return &queueRig{q: q, c: c, r: r, b: b, waits: waits, prepared: prepared}
}

func queuePlan(n int) BulkPlan {
	p := BulkPlan{}
	for i := range n {
		p.Items = append(p.Items, QueueItem{TorrentID: fmt.Sprint(i), TorrentName: "torrent", Target: models.DownloadTarget{Index: i, Label: fmt.Sprintf("file%d", i), Link: fmt.Sprintf("https://example.com/%d", i)}})
	}
	return p
}

func (r *queueRig) start(t *testing.T, n int) QueueID {
	t.Helper()
	id, err := r.q.Start(t.Context(), queuePlan(n))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *queueRig) admit(t *testing.T, index int) startCall {
	t.Helper()
	call := receive(t, r.b.starts)
	if call.req.URL != fmt.Sprintf("https://example.com/%d", index) || call.req.Filename != fmt.Sprintf("file%d", index) {
		t.Fatalf("out of order: %+v", call.req)
	}
	call.reply <- backendResult{download: models.ManagedDownload{GID: fmt.Sprintf("gid%d", index), Status: models.ManagedDownloadStatusActive}}
	return call
}

func queueSnapshot(t *testing.T, q *BulkQueue, id QueueID, predicate func(QueueSnapshot) bool) QueueSnapshot {
	t.Helper()
	// A channel-backed wait below drives work; this loop only observes the
	// published transition, with a bounded failure guard rather than a sleep.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		s, err := q.Snapshot(id)
		if err != nil {
			t.Fatal(err)
		}
		if predicate(s) {
			return s
		}
		select {
		case <-ctx.Done():
			t.Fatalf("snapshot transition timed out: %+v", s)
		default:
		}
		runtime.Gosched()
	}
}

func finishQueue(t *testing.T, r *queueRig, id QueueID) QueueSnapshot {
	t.Helper()
	return queueSnapshot(t, r.q, id, func(s QueueSnapshot) bool { return s.Finished })
}

func TestQueueAdvancesWithoutUI(t *testing.T) {
	r := newQueueRig(t, false)
	serial := &serialQueueBackend{Backend: r.b}
	r.c.backend = serial
	caller, cancel := context.WithCancel(t.Context())
	plan := queuePlan(2)
	id, err := r.q.Start(caller, plan)
	if err != nil {
		t.Fatal(err)
	}
	cancel() // Admission cancellation must not own the admitted queue.
	plan.Items[0].TorrentID = "mutated"
	for i := range 2 {
		r.admit(t, i)
		query := receive(t, r.b.statuses)
		query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	}
	s := finishQueue(t, r, id)
	serial.mu.Lock()
	parallel, order := serial.parallel, slices.Clone(serial.order)
	serial.mu.Unlock()
	if parallel || !slices.Equal(order, []string{"https://example.com/0", "https://example.com/1"}) {
		t.Fatal("queue ran in parallel or out of order", parallel, order)
	}
	if s.Items[0].Item.TorrentID != "0" || s.Items[1].Outcome.Kind != OutcomeSuccess {
		t.Fatal(s)
	}
	s.Items[0].Outcome.Kind = OutcomeFailed
	s.Items[0].Item.TorrentID = "changed"
	s.Download.Download.Filename = "changed"
	again, _ := r.q.Snapshot(id)
	if again.Items[0].Outcome.Kind != OutcomeSuccess || again.Items[0].Item.TorrentID != "0" || again.Download.Download.Filename == "changed" {
		t.Fatal("snapshot aliases queue", again)
	}
}

func TestQueueRejectsForeignActiveDownload(t *testing.T) {
	r := newQueueRig(t, false)
	foreign := admit(t, r.c, r.b, "foreign")
	if _, err := r.q.Start(t.Context(), queuePlan(1)); !errors.Is(err, ErrActiveDownload) {
		t.Fatal(err)
	}
	current, _ := r.c.Current()
	if current.Ref != foreign.Ref {
		t.Fatal("foreign transfer replaced")
	}
	select {
	case <-r.prepared:
		t.Fatal("foreign transfer credited to queue")
	default:
	}
}

func TestQueueLeasePreventsInterleavedStarts(t *testing.T) {
	r := newQueueRig(t, true)
	id := r.start(t, 2)
	queueSnapshot(t, r.q, id, func(s QueueSnapshot) bool { return s.AwaitingOverwrite })
	if _, err := r.c.Start(t.Context(), "foreign", ""); !errors.Is(err, ErrQueueActive) {
		t.Fatal(err)
	}
	if _, err := r.q.Start(t.Context(), queuePlan(1)); !errors.Is(err, ErrQueueActive) {
		t.Fatal(err)
	}
	if err := r.q.Decide(t.Context(), QueueDecision{QueueID: id, ItemIndex: 0, Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	r.admit(t, 0)
	query := receive(t, r.b.statuses)
	active, err := r.c.Start(t.Context(), "foreign", "")
	if err != nil || !active.Reused || active.Snapshot.Ref.GID != "gid0" {
		t.Fatal(active, err)
	}
	query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	queueSnapshot(t, r.q, id, func(s QueueSnapshot) bool { return s.AwaitingOverwrite && s.CurrentIndex == 1 })
	if _, err := r.c.Start(t.Context(), "foreign", ""); !errors.Is(err, ErrQueueActive) {
		t.Fatal(err)
	}
	r.q.Cancel(id)
	finishQueue(t, r, id)
}

func TestQueueDuplicateCompletionAdvancesOnce(t *testing.T) {
	r := newQueueRig(t, false)
	id := r.start(t, 3)
	r.admit(t, 0)
	first := receive(t, r.b.statuses)
	ref, _ := r.c.Current()
	waiter := observeWait(t.Context())
	other := asyncStatus(r.c, waiter, ref.Ref)
	// Register the duplicate before releasing the shared observation. Without
	// this barrier an old query worker can run after gid2's worker, and replying
	// to the presumed "third" query completes gid0 rather than the current item.
	receive(t, waiter.waiting)
	first.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	r.admit(t, 1)
	// The duplicate observation of the old reference cannot finish item 1.
	second := receive(t, r.b.statuses)
	if second.gid != "gid1" {
		t.Fatal(second.gid)
	}
	s, _ := r.q.Snapshot(id)
	if s.CurrentIndex != 1 || s.Items[1].Outcome != nil {
		t.Fatal("duplicate skipped item", s)
	}
	second.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	r.admit(t, 2)
	third := receive(t, r.b.statuses)
	if third.gid != "gid2" {
		t.Fatal(third.gid)
	}
	third.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	receive(t, other)
	finishQueue(t, r, id)
}

func TestQueueStaleDecisionRejected(t *testing.T) {
	r := newQueueRig(t, true)
	id := r.start(t, 2)
	queueSnapshot(t, r.q, id, func(s QueueSnapshot) bool { return s.AwaitingOverwrite })
	for _, d := range []QueueDecision{{QueueID: id + 1, ItemIndex: 0}, {QueueID: id, ItemIndex: 1}} {
		if err := r.q.Decide(t.Context(), d); err == nil {
			t.Fatal("accepted stale decision")
		}
	}
	d := QueueDecision{QueueID: id, ItemIndex: 0}
	if err := r.q.Decide(t.Context(), d); err != nil {
		t.Fatal(err)
	}
	if err := r.q.Decide(t.Context(), d); err == nil {
		t.Fatal("accepted duplicate decision")
	}
	queueSnapshot(t, r.q, id, func(s QueueSnapshot) bool { return s.AwaitingOverwrite && s.CurrentIndex == 1 })
	if err := r.q.Decide(t.Context(), d); err == nil {
		t.Fatal("accepted old item decision")
	}
	r.q.Cancel(id)
	s := finishQueue(t, r, id)
	if s.Items[0].Outcome.Kind != OutcomeSkipped || s.Items[1].Outcome.Kind != OutcomeCanceled {
		t.Fatal(s)
	}
	next := r.start(t, 1)
	if next <= id {
		t.Fatal("queue identity reused")
	}
	r.q.Cancel(next)
	finishQueue(t, r, next)
	if _, err := r.q.Snapshot(id); err == nil {
		t.Fatal("old queue snapshot accepted")
	}
}

func TestQueueCancelUnblocksOverwrite(t *testing.T) {
	for _, root := range []bool{false, true} {
		t.Run(fmt.Sprint(root), func(t *testing.T) {
			r := newQueueRig(t, true)
			id := r.start(t, 2)
			queueSnapshot(t, r.q, id, func(s QueueSnapshot) bool { return s.AwaitingOverwrite })
			if root {
				r.r.Stop()
			} else if err := r.q.Cancel(id); err != nil {
				t.Fatal(err)
			}
			s := finishQueue(t, r, id)
			for _, item := range s.Items {
				if item.Status != "terminal" || item.Outcome == nil || item.Outcome.Kind != OutcomeCanceled {
					t.Fatal(s)
				}
			}
			if s.AwaitingOverwrite {
				t.Fatal(s)
			}
		})
	}
}

func TestQueueCancelWaitsForTransferStop(t *testing.T) {
	r := newQueueRig(t, false)
	id := r.start(t, 2)
	call := r.admit(t, 0)
	query := receive(t, r.b.statuses)
	if err := r.q.Cancel(id); err != nil {
		t.Fatal(err)
	}
	receive(t, call.ctx.Done())
	s, _ := r.q.Snapshot(id)
	if s.Finished {
		t.Fatal("finished before stop acknowledged")
	}
	if _, err := r.c.AcquireQueue(t.Context()); !errors.Is(err, ErrQueueActive) {
		t.Fatal("lease freed before acknowledgment", err)
	}
	if _, err := r.c.Start(t.Context(), "foreign", ""); err != nil {
		t.Fatal("must reopen owned active transfer", err)
	}
	query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusRemoved}}
	s = finishQueue(t, r, id)
	if s.Items[0].Outcome.Kind != OutcomeCanceled || s.Items[1].Outcome.Kind != OutcomeCanceled {
		t.Fatal(s)
	}
	lease, err := r.c.AcquireQueue(t.Context())
	if err != nil {
		t.Fatal("lease not released", err)
	}
	if err := r.c.ReleaseQueue(lease); err != nil {
		t.Fatal(err)
	}
}

func TestQueueUncertainStartCancelsRemaining(t *testing.T) {
	r := newQueueRig(t, false)
	id := r.start(t, 2)
	call := receive(t, r.b.starts)
	call.reply <- backendResult{err: download.ErrUncertainStart}
	s := finishQueue(t, r, id)
	if s.Items[0].Outcome.Kind != OutcomeFailed || !errors.Is(s.Items[0].Outcome.Err, download.ErrUncertainStart) || !s.Items[0].Outcome.RemoteOutcomeUnknown || s.Items[1].Outcome.Kind != OutcomeCanceled {
		t.Fatal(s)
	}
	receive(t, r.b.shutdown)
	if _, err := r.c.Start(t.Context(), "again", ""); !errors.Is(err, download.ErrUncertainStart) {
		t.Fatal(err)
	}
	if r.b.startCalls.Load() != 1 {
		t.Fatal("retried uncertain admission")
	}
}

func TestQueueStopUnconfirmedRequiresRestart(t *testing.T) {
	r := newQueueRig(t, false)
	lease, err := r.c.AcquireQueue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan startResult, 1)
	go func() { s, e := r.c.StartQueued(t.Context(), lease, "url", "name"); results <- startResult{s, e} }()
	call := receive(t, r.b.starts)
	call.reply <- backendResult{download: models.ManagedDownload{GID: "stuck", Status: models.ManagedDownloadStatusActive}}
	started := receive(t, results)
	if err := r.c.ReleaseQueue(lease); !errors.Is(err, ErrTransferStopUnconfirmed) {
		t.Fatal("released active lease", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := r.c.CancelQueued(ctx, lease, started.start.Snapshot.Ref); !errors.Is(err, ErrTransferStopUnconfirmed) {
		t.Fatal(err)
	}
	receive(t, call.ctx.Done())
	receive(t, r.b.shutdown)
	if _, err := r.c.Start(t.Context(), "again", ""); !errors.Is(err, ErrTransferStopUnconfirmed) {
		t.Fatal(err)
	}
	if _, err := r.c.AcquireQueue(t.Context()); !errors.Is(err, ErrTransferStopUnconfirmed) {
		t.Fatal(err)
	}
}

func TestQueueStatusTransportErrorRetries(t *testing.T) {
	r := newQueueRig(t, false)
	id := r.start(t, 2)
	r.admit(t, 0)
	query := receive(t, r.b.statuses)
	query.reply <- backendResult{err: errors.New("temporary transport failure")}
	gate := receive(t, r.waits)
	s, _ := r.q.Snapshot(id)
	if s.CurrentIndex != 0 || s.Items[0].Outcome != nil || s.Download.Download.Status != models.ManagedDownloadStatusActive {
		t.Fatal(s)
	}
	if r.b.startCalls.Load() != 1 {
		t.Fatal("query failure started a second transfer")
	}
	close(gate)
	query = receive(t, r.b.statuses)
	query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusError, ErrorMessage: "definitive failure"}}
	r.admit(t, 1)
	query = receive(t, r.b.statuses)
	query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	s = finishQueue(t, r, id)
	if s.Items[0].Outcome.Kind != OutcomeFailed || s.Items[1].Outcome.Kind != OutcomeSuccess {
		t.Fatal(s)
	}
}

func TestQueuePublishesProgressBeforeNextPoll(t *testing.T) {
	r := newQueueRig(t, false)
	id := r.start(t, 1)
	r.admit(t, 0)
	query := receive(t, r.b.statuses)
	query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusActive, CompletedLength: 50, TotalLength: 100}}
	gate := receive(t, r.waits)
	s, _ := r.q.Snapshot(id)
	if s.Download.Download.CompletedLength != 50 || s.Download.Download.TotalLength != 100 {
		t.Fatalf("progress not published before wait: %+v", s.Download)
	}
	close(gate)
	query = receive(t, r.b.statuses)
	query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	finishQueue(t, r, id)
}

type queueStartBackend struct {
	download.Backend
	start func(context.Context, models.ManagedDownloadRequest) (models.ManagedDownload, error)
}

func (b queueStartBackend) StartDownload(ctx context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
	return b.start(ctx, req)
}

func TestQueueUncertainStartWinsCancellationRace(t *testing.T) {
	r := newQueueRig(t, false)
	ids := make(chan QueueID, 1)
	r.c.backend = queueStartBackend{Backend: r.b, start: func(ctx context.Context, _ models.ManagedDownloadRequest) (models.ManagedDownload, error) {
		id := <-ids
		if err := r.q.Cancel(id); err != nil {
			return models.ManagedDownload{}, err
		}
		// Simulate an AddURI transport failure whose response arrives after the
		// queue has canceled its admission waiter. The outcome is still unknown.
		return models.ManagedDownload{}, download.ErrUncertainStart
	}}
	id := r.start(t, 2)
	ids <- id
	s := finishQueue(t, r, id)
	if s.Items[0].Outcome.Kind != OutcomeFailed || !s.Items[0].Outcome.RemoteOutcomeUnknown || !errors.Is(s.Items[0].Outcome.Err, download.ErrUncertainStart) || s.Items[1].Outcome.Kind != OutcomeCanceled {
		t.Fatal(s.Items)
	}
}

func TestQueueCancelDuringStart(t *testing.T) {
	r := newQueueRig(t, false)
	id := r.start(t, 2)
	call := receive(t, r.b.starts)
	if err := r.q.Cancel(id); err != nil {
		t.Fatal(err)
	}
	receive(t, call.ctx.Done())
	s := finishQueue(t, r, id)
	if s.Items[0].Outcome.Kind != OutcomeCanceled || s.Items[1].Outcome.Kind != OutcomeCanceled {
		t.Fatal(s)
	}
	lease, err := r.c.AcquireQueue(t.Context())
	if err != nil {
		t.Fatal("canceled admission leaked lease", err)
	}
	r.c.ReleaseQueue(lease)
}

func TestQueuePreparationAndRejectedStartContinue(t *testing.T) {
	r := newQueueRig(t, false)
	original := r.q.deps.Prepare
	want := errors.New("resolve failure")
	r.q.deps.Prepare = func(ctx context.Context, target models.DownloadTarget) (PreparedDownload, error) {
		if target.Index == 0 {
			return PreparedDownload{}, want
		}
		return original(ctx, target)
	}
	id := r.start(t, 3)
	call := receive(t, r.b.starts)
	if call.req.Filename != "file1" {
		t.Fatal(call.req)
	}
	call.reply <- backendResult{err: errors.New("explicit rejection")}
	r.admit(t, 2)
	query := receive(t, r.b.statuses)
	query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	s := finishQueue(t, r, id)
	if !errors.Is(s.Items[0].Outcome.Err, want) || s.Items[1].Outcome.Kind != OutcomeFailed || s.Items[2].Outcome.Kind != OutcomeSuccess {
		t.Fatal(s)
	}
}

func TestQueueCanceledAdmissionAndRuntimeRejection(t *testing.T) {
	r := newQueueRig(t, false)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.q.Start(ctx, queuePlan(1)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	r.r.Stop()
	if _, err := r.q.Start(t.Context(), queuePlan(1)); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	lease, err := r.c.AcquireQueue(t.Context())
	if err != nil {
		t.Fatal("runtime rejection leaked lease", err)
	}
	r.c.ReleaseQueue(lease)
}
