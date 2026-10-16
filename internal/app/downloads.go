package app

import (
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/pkg/models"
)

var ErrStaleDownload = errors.New("download reference is no longer current")

var (
	ErrActiveDownload          = errors.New("an unrelated download is active")
	ErrQueueActive             = errors.New("a bulk queue owns download admission")
	ErrStaleQueue              = errors.New("queue identity or decision is no longer current")
	ErrTransferStopUnconfirmed = errors.New("download termination is unconfirmed; restart the application")
)

type QueueID uint64
type QueueLease struct{ ID QueueID }

// DownloadRef identifies an admission even when a backend reuses its GID.
type DownloadRef struct {
	Generation uint64
	GID        string
}

type DownloadSnapshot struct {
	Ref      DownloadRef
	Download models.ManagedDownload
}

type DownloadStart struct {
	Snapshot DownloadSnapshot
	Reused   bool
}

// DownloadCoordinator owns admission, transfer lifetimes and cached snapshots.
// Request contexts only bound waiting; parent owns the application lifetime.
type DownloadCoordinator struct {
	mu         sync.Mutex
	backend    download.Backend
	dir        string
	ctx        context.Context
	cancel     context.CancelFunc
	stopParent func() bool
	wg         sync.WaitGroup
	done       chan struct{}
	closing    bool
	fault      error
	closeErr   error
	generation uint64
	current    *DownloadSnapshot
	transfer   context.CancelFunc
	start      *startReservation
	query      *statusQuery
	nextQueue  QueueID
	lease      QueueLease
	owner      QueueID
}

type startReservation struct {
	generation uint64
	done       chan struct{}
	snapshot   DownloadSnapshot
	err        error
	owner      QueueID
	cancel     context.CancelFunc
}

type statusQuery struct {
	ref      DownloadRef
	done     chan struct{}
	snapshot DownloadSnapshot
	err      error
}

func NewDownloadCoordinator(parent context.Context, backend download.Backend, dir string) *DownloadCoordinator {
	ctx, cancel := context.WithCancel(parent)
	c := &DownloadCoordinator{backend: backend, dir: dir, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	// Serialize callback registration with closure, including an already canceled parent.
	c.mu.Lock()
	c.stopParent = context.AfterFunc(parent, func() {
		c.mu.Lock()
		c.beginCloseLocked()
		c.mu.Unlock()
	})
	c.mu.Unlock()
	return c
}

func (c *DownloadCoordinator) Start(ctx context.Context, url, filename string) (DownloadStart, error) {
	return c.startDownload(ctx, QueueLease{}, url, filename)
}

func (c *DownloadCoordinator) StartQueued(ctx context.Context, lease QueueLease, url, filename string) (DownloadStart, error) {
	if lease.ID == 0 {
		return DownloadStart{}, ErrStaleQueue
	}
	return c.startDownload(ctx, lease, url, filename)
}

func (c *DownloadCoordinator) startDownload(ctx context.Context, lease QueueLease, url, filename string) (DownloadStart, error) {
	if err := ctx.Err(); err != nil {
		return DownloadStart{}, err
	}
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return DownloadStart{}, err
	}
	if c.closing || c.ctx.Err() != nil {
		c.beginCloseLocked()
		err := c.closedErrorLocked()
		c.mu.Unlock()
		return DownloadStart{}, err
	}
	if lease.ID != 0 && c.lease != lease {
		c.mu.Unlock()
		return DownloadStart{}, ErrStaleQueue
	}
	reservation, reused := c.start, true
	if reservation == nil {
		if c.current != nil && !c.current.Download.IsTerminal() {
			result := DownloadStart{Snapshot: *c.current, Reused: true}
			c.mu.Unlock()
			return result, nil
		}
		if lease.ID == 0 && c.lease.ID != 0 {
			c.mu.Unlock()
			return DownloadStart{}, ErrQueueActive
		}
		c.generation++
		transferCtx, cancel := context.WithCancel(c.ctx)
		reservation = &startReservation{generation: c.generation, done: make(chan struct{}), owner: lease.ID, cancel: cancel}
		c.start = reservation
		reused = false
		request := models.ManagedDownloadRequest{URL: url, Dir: c.dir, Filename: downloadFilename(url, filename)}
		c.wg.Go(func() { c.runStart(transferCtx, cancel, reservation, request) })
	}
	c.mu.Unlock()
	select {
	case <-reservation.done:
		return DownloadStart{Snapshot: reservation.snapshot, Reused: reused}, reservation.err
	case <-ctx.Done():
		return DownloadStart{}, ctx.Err()
	case <-c.ctx.Done():
		return DownloadStart{}, c.closedError()
	}
}

func (c *DownloadCoordinator) runStart(ctx context.Context, cancel context.CancelFunc, reservation *startReservation, request models.ManagedDownloadRequest) {
	started, err := c.backend.StartDownload(ctx, request)
	c.mu.Lock()
	defer c.mu.Unlock()
	defer close(reservation.done)
	if errors.Is(err, download.ErrUncertainStart) {
		c.fault = err
		c.beginCloseLocked()
	} else if c.closing || c.ctx.Err() != nil {
		err = c.closedErrorLocked()
	}
	if err != nil {
		cancel()
		reservation.err = err
	} else if c.start == reservation {
		started = mergeDownload(started, models.ManagedDownload{
			URL: request.URL, Filename: request.Filename, Directory: request.Dir,
			FilePath: filepath.Join(request.Dir, request.Filename), Status: models.ManagedDownloadStatusWaiting,
		})
		reservation.snapshot = DownloadSnapshot{Ref: DownloadRef{Generation: reservation.generation, GID: started.GID}, Download: started}
		c.current = new(reservation.snapshot)
		c.transfer = cancel
		c.owner = reservation.owner
		if started.IsTerminal() {
			cancel()
		}
	}
	if c.start == reservation {
		c.start = nil
	}
}

// AcquireQueue reserves admission across resolution, confirmation and item gaps.
func (c *DownloadCoordinator) AcquireQueue(ctx context.Context) (QueueLease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return QueueLease{}, err
	}
	if c.closing || c.ctx.Err() != nil {
		c.beginCloseLocked()
		return QueueLease{}, c.closedErrorLocked()
	}
	if c.lease.ID != 0 {
		return QueueLease{}, ErrQueueActive
	}
	if c.start != nil || (c.current != nil && !c.current.Download.IsTerminal()) {
		return QueueLease{}, ErrActiveDownload
	}
	c.nextQueue++
	c.lease = QueueLease{ID: c.nextQueue}
	return c.lease, nil
}

func (c *DownloadCoordinator) ReleaseQueue(lease QueueLease) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if lease.ID == 0 || c.lease != lease {
		return ErrStaleQueue
	}
	if errors.Is(c.fault, ErrTransferStopUnconfirmed) || c.start != nil || (c.current != nil && !c.current.Download.IsTerminal()) {
		return ErrTransferStopUnconfirmed
	}
	c.lease = QueueLease{}
	return nil
}

// CancelQueued retains the lease until a terminal observation proves that the
// owned transfer stopped. A zero ref also covers cancellation during admission.
func (c *DownloadCoordinator) CancelQueued(ctx context.Context, lease QueueLease, ref DownloadRef) error {
	c.mu.Lock()
	if lease.ID == 0 || c.lease != lease {
		c.mu.Unlock()
		return ErrStaleQueue
	}
	reservation := c.start
	if ref != (DownloadRef{}) && (c.current == nil || c.current.Ref != ref || c.owner != lease.ID || (reservation != nil && reservation.generation != ref.Generation)) {
		c.mu.Unlock()
		return ErrStaleDownload
	}
	if reservation != nil && reservation.owner == lease.ID {
		reservation.cancel()
	} else if c.current != nil && c.owner == lease.ID && c.transfer != nil {
		c.transfer()
	}
	c.mu.Unlock()
	if reservation != nil {
		select {
		case <-reservation.done:
		case <-ctx.Done():
			return c.stopUnconfirmed(ctx.Err())
		}
	}
	c.mu.Lock()
	if errors.Is(c.fault, download.ErrUncertainStart) {
		err := c.fault
		c.mu.Unlock()
		return err
	}
	if c.current == nil || c.owner != lease.ID || c.current.Download.IsTerminal() {
		c.mu.Unlock()
		return nil
	}
	ref = c.current.Ref
	c.mu.Unlock()
	for {
		snapshot, err := c.Status(ctx, ref)
		if snapshot.Ref == ref && snapshot.Download.IsTerminal() {
			return nil
		}
		if err != nil && (ctx.Err() != nil || errors.Is(err, download.ErrClosed) || errors.Is(err, ErrStaleDownload) || errors.Is(err, download.ErrUncertainStart) || errors.Is(err, ErrTransferStopUnconfirmed)) {
			return c.stopUnconfirmed(err)
		}
		if err := waitBatch(ctx, time.Second); err != nil {
			return c.stopUnconfirmed(err)
		}
	}
}

func (c *DownloadCoordinator) stopUnconfirmed(cause error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fault = errors.Join(ErrTransferStopUnconfirmed, c.fault, cause)
	c.beginCloseLocked()
	return c.fault
}

func (c *DownloadCoordinator) Current() (DownloadSnapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return DownloadSnapshot{}, false
	}
	return *c.current, true
}

func (c *DownloadCoordinator) Status(ctx context.Context, ref DownloadRef) (DownloadSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return DownloadSnapshot{}, err
	}
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return DownloadSnapshot{}, err
	}
	if c.closing || c.ctx.Err() != nil {
		c.beginCloseLocked()
		err := c.closedErrorLocked()
		c.mu.Unlock()
		return DownloadSnapshot{}, err
	}
	if c.current == nil || c.current.Ref != ref {
		c.mu.Unlock()
		return DownloadSnapshot{}, ErrStaleDownload
	}
	query := c.query
	if query == nil || query.ref != ref {
		query = &statusQuery{ref: ref, done: make(chan struct{})}
		c.query = query
		c.wg.Go(func() { c.runStatus(query) })
	}
	c.mu.Unlock()
	select {
	case <-query.done:
		return query.snapshot, query.err
	case <-ctx.Done():
		return DownloadSnapshot{}, ctx.Err()
	case <-c.ctx.Done():
		return DownloadSnapshot{}, c.closedError()
	}
}

func (c *DownloadCoordinator) runStatus(query *statusQuery) {
	observed, err := c.backend.DownloadStatus(c.ctx, query.ref.GID)
	c.mu.Lock()
	defer c.mu.Unlock()
	defer close(query.done)
	if c.query == query {
		c.query = nil
	}
	if c.closing || c.ctx.Err() != nil {
		query.err = c.closedErrorLocked()
		return
	}
	if c.current == nil || c.current.Ref != query.ref {
		query.err = ErrStaleDownload
		return
	}
	query.snapshot = *c.current
	query.err = err
	if err == nil && !c.current.Download.IsTerminal() {
		observed = mergeDownload(observed, c.current.Download)
		observed.GID = query.ref.GID
		query.snapshot.Download = observed
	} else if errors.Is(err, download.ErrBackendLost) && !c.current.Download.IsTerminal() {
		query.snapshot.Download.Status = models.ManagedDownloadStatusError
		query.snapshot.Download.ErrorMessage = err.Error()
	}
	// Transient query errors preserve the entire last valid snapshot.
	c.current = new(query.snapshot)
	if c.current.Download.IsTerminal() && c.transfer != nil {
		c.transfer()
	}
}

func mergeDownload(observed, known models.ManagedDownload) models.ManagedDownload {
	observed.URL = cmp.Or(observed.URL, known.URL)
	observed.Filename = cmp.Or(observed.Filename, known.Filename)
	observed.Directory = cmp.Or(observed.Directory, known.Directory)
	observed.FilePath = cmp.Or(observed.FilePath, known.FilePath)
	observed.Status = cmp.Or(observed.Status, known.Status)
	return observed
}

func (c *DownloadCoordinator) Close(ctx context.Context) error {
	c.mu.Lock()
	c.beginCloseLocked()
	c.mu.Unlock()
	select {
	case <-c.done:
		return c.closeErr
	default:
	}
	select {
	case <-c.done:
		return c.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// All workers are registered under mu before the shared join begins. Backend I/O
// never holds mu; a Close caller's deadline cannot abandon the owned cleanup.
func (c *DownloadCoordinator) beginCloseLocked() {
	if c.closing {
		return
	}
	c.closing = true
	c.stopParent()
	c.cancel()
	c.wg.Go(func() {
		err := c.backend.Shutdown(context.Background())
		c.mu.Lock()
		c.closeErr = err
		c.mu.Unlock()
	})
	go func() {
		c.wg.Wait()
		close(c.done)
	}()
}

func (c *DownloadCoordinator) closedError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closedErrorLocked()
}

func (c *DownloadCoordinator) closedErrorLocked() error {
	if c.fault != nil {
		return c.fault
	}
	return download.ErrClosed
}
