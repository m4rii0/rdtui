package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/pkg/models"
)

// These barriers catch duplicate admissions/queries without scheduler timing.
type coordinatorBackend struct {
	startCalls, statusCalls, shutdownCalls atomic.Int64
	starts                                 chan startCall
	statuses                               chan statusCall
	shutdown                               chan struct{}
	shutdownGate                           <-chan struct{}
	shutdownOnce                           sync.Once
}

func TestStartReopensActiveWithoutWaitingForObservation(t *testing.T) {
	for _, name := range []string{"in-flight-single-observation", "queue-before-observation", "failed-single-observation"} {
		t.Run(name, func(t *testing.T) {
			queued := name == "queue-before-observation"
			b := newCoordinatorBackend()
			c := testCoordinator(t, b)
			var current DownloadSnapshot
			var observed <-chan statusResult
			var query statusCall
			if queued {
				lease, err := c.AcquireQueue(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				results := make(chan startResult, 1)
				go func() {
					started, err := c.StartQueued(t.Context(), lease, "queued", "queued")
					results <- startResult{started, err}
				}()
				call := receive(t, b.starts)
				call.reply <- backendResult{download: models.ManagedDownload{GID: "queued", Status: models.ManagedDownloadStatusActive}}
				result := receive(t, results)
				if result.err != nil {
					t.Fatal(result.err)
				}
				current = result.start.Snapshot
			} else {
				current = admit(t, c, b, "single")
				observed = asyncStatus(c, t.Context(), current.Ref)
				query = receive(t, b.statuses)
				if name == "failed-single-observation" {
					query.reply <- backendResult{err: errors.New("status transport failure")}
					receive(t, observed)
					observed = nil
				}
			}
			// The observation is deliberately withheld until Start has reopened
			// the existing transfer. Reuse must not depend on remote observation.
			result := receive(t, asyncStart(c, t.Context()))
			if result.err != nil || !result.start.Reused || result.start.Snapshot.Ref != current.Ref {
				t.Fatalf("reopen: %+v", result)
			}
			if b.startCalls.Load() != 1 {
				t.Fatal("reopen started another transfer")
			}
			if observed != nil {
				query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusActive}}
				receive(t, observed)
			}
		})
	}
}

func TestQueueTerminalObservationCannotResurrectTransfer(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	old := admit(t, c, b, "old")
	result := asyncStatus(c, t.Context(), old.Ref)
	call := receive(t, b.statuses)
	call.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	receive(t, result)
	result = asyncStatus(c, t.Context(), old.Ref)
	call = receive(t, b.statuses)
	call.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusActive}}
	observed := receive(t, result)
	if !observed.snapshot.Download.IsComplete() {
		t.Fatal("terminal transfer resurrected", observed)
	}
	lease, err := c.AcquireQueue(t.Context())
	if err != nil {
		t.Fatal("old observation blocked queue", err)
	}
	if err := c.ReleaseQueue(lease); err != nil {
		t.Fatal(err)
	}
}

func TestQueueLeaseIdentityRejectsForeignControl(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	lease, err := c.AcquireQueue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	foreign := QueueLease{ID: lease.ID + 1}
	for _, bad := range []QueueLease{{}, foreign} {
		if _, err := c.StartQueued(t.Context(), bad, "foreign", ""); !errors.Is(err, ErrStaleQueue) {
			t.Fatal(err)
		}
		if err := c.CancelQueued(t.Context(), bad, DownloadRef{}); !errors.Is(err, ErrStaleQueue) {
			t.Fatal(err)
		}
		if err := c.ReleaseQueue(bad); !errors.Is(err, ErrStaleQueue) {
			t.Fatal(err)
		}
	}
	results := make(chan startResult, 1)
	go func() { s, e := c.StartQueued(t.Context(), lease, "url", "file"); results <- startResult{s, e} }()
	call := receive(t, b.starts)
	if _, err := c.AcquireQueue(t.Context()); !errors.Is(err, ErrQueueActive) {
		t.Fatal(err)
	}
	call.reply <- backendResult{download: models.ManagedDownload{GID: "owned", Status: models.ManagedDownloadStatusActive}}
	started := receive(t, results)
	wrongRef := started.start.Snapshot.Ref
	wrongRef.Generation++
	if err := c.CancelQueued(t.Context(), lease, wrongRef); !errors.Is(err, ErrStaleDownload) {
		t.Fatal(err)
	}
	if call.ctx.Err() != nil {
		t.Fatal("foreign reference canceled owned transfer")
	}
	result := asyncStatus(c, t.Context(), started.start.Snapshot.Ref)
	query := receive(t, b.statuses)
	query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	receive(t, result)
	if err := c.ReleaseQueue(lease); err != nil {
		t.Fatal(err)
	}
	next, err := c.AcquireQueue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if next.ID <= lease.ID {
		t.Fatal("lease identity reused")
	}
	if err := c.ReleaseQueue(lease); !errors.Is(err, ErrStaleQueue) {
		t.Fatal(err)
	}
	if err := c.ReleaseQueue(next); err != nil {
		t.Fatal(err)
	}
}

func TestQueueOldReferenceCannotCancelNextAdmission(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	lease, err := c.AcquireQueue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	start := func() <-chan startResult {
		results := make(chan startResult, 1)
		go func() { s, e := c.StartQueued(t.Context(), lease, "url", "file"); results <- startResult{s, e} }()
		return results
	}
	results := start()
	call := receive(t, b.starts)
	call.reply <- backendResult{download: models.ManagedDownload{GID: "old", Status: models.ManagedDownloadStatusComplete}}
	old := receive(t, results).start.Snapshot.Ref
	results = start()
	call = receive(t, b.starts)
	if err := c.CancelQueued(t.Context(), lease, old); !errors.Is(err, ErrStaleDownload) {
		t.Fatal("old ref controlled next admission", err)
	}
	if call.ctx.Err() != nil {
		t.Fatal("old reference canceled next transfer")
	}
	call.reply <- backendResult{download: models.ManagedDownload{GID: "new", Status: models.ManagedDownloadStatusComplete}}
	if result := receive(t, results); result.err != nil {
		t.Fatal(result)
	}
	if err := c.ReleaseQueue(lease); err != nil {
		t.Fatal(err)
	}
}

type backendResult struct {
	download models.ManagedDownload
	err      error
}

type startCall struct {
	ctx   context.Context
	req   models.ManagedDownloadRequest
	reply chan backendResult
}

type statusCall struct {
	ctx   context.Context
	gid   string
	reply chan backendResult
}

func newCoordinatorBackend() *coordinatorBackend {
	return &coordinatorBackend{starts: make(chan startCall, 16), statuses: make(chan statusCall, 16), shutdown: make(chan struct{})}
}

func (b *coordinatorBackend) StartDownload(ctx context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
	b.startCalls.Add(1)
	call := startCall{ctx, req, make(chan backendResult, 1)}
	b.starts <- call
	select {
	case r := <-call.reply:
		return r.download, r.err
	case <-ctx.Done():
		return models.ManagedDownload{}, ctx.Err()
	}
}

func (b *coordinatorBackend) DownloadStatus(ctx context.Context, gid string) (models.ManagedDownload, error) {
	b.statusCalls.Add(1)
	call := statusCall{ctx, gid, make(chan backendResult, 1)}
	b.statuses <- call
	select {
	case r := <-call.reply:
		return r.download, r.err
	case <-ctx.Done():
		return models.ManagedDownload{}, ctx.Err()
	}
}

func (b *coordinatorBackend) Shutdown(ctx context.Context) error {
	b.shutdownCalls.Add(1)
	b.shutdownOnce.Do(func() { close(b.shutdown) })
	if b.shutdownGate != nil {
		select {
		case <-b.shutdownGate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("barrier timed out")
		var zero T
		return zero
	}
}

// Done is observed only once a request is waiting on the shared operation.
type waitingContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func observeWait(ctx context.Context) *waitingContext {
	return &waitingContext{Context: ctx, waiting: make(chan struct{})}
}

func (ctx *waitingContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func testCoordinator(t *testing.T, b *coordinatorBackend) *DownloadCoordinator {
	t.Helper()
	c := NewDownloadCoordinator(t.Context(), b, "/downloads")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.Close(ctx); err != nil {
			t.Errorf("coordinator cleanup: %v", err)
		}
	})
	return c
}

type startResult struct {
	start DownloadStart
	err   error
}

func asyncStart(c *DownloadCoordinator, ctx context.Context) <-chan startResult {
	results := make(chan startResult, 1)
	go func() {
		start, err := c.Start(ctx, "https://example.com/file.zip", "")
		results <- startResult{start, err}
	}()
	return results
}

func admit(t *testing.T, c *DownloadCoordinator, b *coordinatorBackend, gid string) DownloadSnapshot {
	t.Helper()
	result := asyncStart(c, t.Context())
	call := receive(t, b.starts)
	if call.req != (models.ManagedDownloadRequest{URL: "https://example.com/file.zip", Dir: "/downloads", Filename: "file.zip"}) {
		t.Fatalf("request = %+v", call.req)
	}
	call.reply <- backendResult{download: models.ManagedDownload{GID: gid, Status: models.ManagedDownloadStatusActive}}
	r := receive(t, result)
	if r.err != nil || r.start.Reused {
		t.Fatalf("admission = %+v", r)
	}
	return r.start.Snapshot
}

type statusResult struct {
	snapshot DownloadSnapshot
	err      error
}

func asyncStatus(c *DownloadCoordinator, ctx context.Context, ref DownloadRef) <-chan statusResult {
	results := make(chan statusResult, 1)
	go func() {
		snapshot, err := c.Status(ctx, ref)
		results <- statusResult{snapshot, err}
	}()
	return results
}

func TestConcurrentStartsShareReservation(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	first := asyncStart(c, t.Context())
	call := receive(t, b.starts)
	waiter := observeWait(t.Context())
	second := asyncStart(c, waiter)
	receive(t, waiter.waiting)
	call.reply <- backendResult{download: models.ManagedDownload{GID: "gid-1", Status: models.ManagedDownloadStatusActive}}
	a, z := receive(t, first), receive(t, second)
	if a.err != nil || z.err != nil || a.start.Reused || !z.start.Reused || a.start.Snapshot.Ref != z.start.Snapshot.Ref {
		t.Fatalf("reuse identity: first=%+v second=%+v", a, z)
	}
	if b.startCalls.Load() != 1 {
		t.Fatal("duplicate transfer")
	}
	if a.start.Snapshot.Ref.Generation == 0 || a.start.Snapshot.Ref.GID != "gid-1" {
		t.Fatalf("reference = %+v", a.start.Snapshot.Ref)
	}
	// Returned values must not alias coordinator metadata.
	a.start.Snapshot.Download.Filename = "changed"
	current, ok := c.Current()
	if !ok || current.Download.Filename != "file.zip" {
		t.Fatalf("mutable snapshot: %+v", current)
	}
}

func TestConcurrentStatusQueriesCoalesce(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	snapshot := admit(t, c, b, "gid-1")
	first := asyncStatus(c, t.Context(), snapshot.Ref)
	call := receive(t, b.statuses)
	if call.gid != "gid-1" {
		t.Fatalf("queried GID = %q", call.gid)
	}
	waiter := observeWait(t.Context())
	second := asyncStatus(c, waiter, snapshot.Ref)
	receive(t, waiter.waiting)
	call.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusActive, CompletedLength: 42}}
	a, z := receive(t, first), receive(t, second)
	if a.err != nil || z.err != nil || a.snapshot != z.snapshot || a.snapshot.Download.CompletedLength != 42 {
		t.Fatalf("coalesced results: %+v %+v", a, z)
	}
	if a.snapshot.Download.GID != "gid-1" || a.snapshot.Download.URL != "https://example.com/file.zip" || a.snapshot.Download.Filename != "file.zip" || a.snapshot.Download.Directory != "/downloads" || a.snapshot.Download.FilePath != filepath.FromSlash("/downloads/file.zip") {
		t.Fatalf("lost metadata: %+v", a.snapshot)
	}
	if b.statusCalls.Load() != 1 {
		t.Fatal("overlapping status queries")
	}
}

func TestStatusFailureDoesNotReleaseActiveSlot(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	snapshot := admit(t, c, b, "gid-1")
	result := asyncStatus(c, t.Context(), snapshot.Ref)
	call := receive(t, b.statuses)
	transient := errors.New("temporary transport failure")
	call.reply <- backendResult{err: transient}
	r := receive(t, result)
	if !errors.Is(r.err, transient) || r.snapshot != snapshot {
		t.Fatalf("transient result = %+v", r)
	}
	current, _ := c.Current()
	if current != snapshot {
		t.Fatalf("failure changed snapshot: %+v", current)
	}
	reused, err := c.Start(t.Context(), "https://example.com/other", "other")
	if err != nil || !reused.Reused || reused.Snapshot.Ref != snapshot.Ref || b.startCalls.Load() != 1 {
		t.Fatalf("transient failure admitted another transfer: %+v, %v", reused, err)
	}
	// A later poll can recover and release the slot on confirmed completion.
	result = asyncStatus(c, t.Context(), snapshot.Ref)
	call = receive(t, b.statuses)
	call.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	if r := receive(t, result); r.err != nil || !r.snapshot.Download.IsComplete() {
		t.Fatalf("recovery = %+v", r)
	}
	replacement := admit(t, c, b, "gid-2")
	if replacement.Ref.Generation <= snapshot.Ref.Generation {
		t.Fatal("generation did not advance")
	}
}

func TestLateStatusCannotOverwriteCurrent(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	old := admit(t, c, b, "gid-1")
	terminal := asyncStatus(c, t.Context(), old.Ref)
	call := receive(t, b.statuses)
	call.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	if r := receive(t, terminal); r.err != nil {
		t.Fatal(r.err)
	}
	late := asyncStatus(c, t.Context(), old.Ref)
	call = receive(t, b.statuses)
	// Reuse the backend GID to prove generation, not GID alone, is checked.
	replacement := admit(t, c, b, "gid-1")
	call.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusError, Filename: "stale"}}
	if r := receive(t, late); !errors.Is(r.err, ErrStaleDownload) {
		t.Fatalf("late result = %+v", r)
	}
	current, _ := c.Current()
	if current != replacement {
		t.Fatal("stale response replaced current")
	}
	if _, err := c.Status(t.Context(), old.Ref); !errors.Is(err, ErrStaleDownload) {
		t.Fatalf("old reference accepted: %v", err)
	}
}

func TestCanceledStartWaiterDoesNotCancelTransfer(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	ctx, cancel := context.WithCancel(t.Context())
	first := asyncStart(c, ctx)
	call := receive(t, b.starts)
	joined := observeWait(t.Context())
	second := asyncStart(c, joined)
	receive(t, joined.waiting)
	cancel()
	if r := receive(t, first); !errors.Is(r.err, context.Canceled) {
		t.Fatalf("canceled waiter = %+v", r)
	}
	if err := call.ctx.Err(); err != nil {
		t.Fatalf("caller canceled transfer: %v", err)
	}
	call.reply <- backendResult{download: models.ManagedDownload{GID: "gid-1", Status: models.ManagedDownloadStatusActive}}
	if r := receive(t, second); r.err != nil || !r.start.Reused || r.start.Snapshot.Ref.GID != "gid-1" {
		t.Fatalf("remaining waiter = %+v", r)
	}
	if call.ctx.Err() != nil || b.startCalls.Load() != 1 {
		t.Fatal("transfer lost with first waiter")
	}
}

func TestCoordinatorCloseDuringStart(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	result := asyncStart(c, t.Context())
	call := receive(t, b.starts)
	if err := c.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r := receive(t, result); !errors.Is(r.err, download.ErrClosed) {
		t.Fatalf("start after close = %+v", r)
	}
	if call.ctx.Err() == nil || b.shutdownCalls.Load() != 1 {
		t.Fatal("close did not terminate owned work")
	}
	if _, ok := c.Current(); ok {
		t.Fatal("closed reservation published")
	}
	if _, err := c.Start(t.Context(), "ignored", "ignored"); !errors.Is(err, download.ErrClosed) {
		t.Fatalf("admission after close: %v", err)
	}
}

func TestUncertainStartRequiresRestart(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	result := asyncStart(c, t.Context())
	call := receive(t, b.starts)
	call.reply <- backendResult{err: download.ErrUncertainStart}
	if r := receive(t, result); !errors.Is(r.err, download.ErrUncertainStart) {
		t.Fatalf("uncertain result = %+v", r)
	}
	receive(t, b.shutdown)
	if _, err := c.Start(t.Context(), "ignored", "ignored"); !errors.Is(err, download.ErrUncertainStart) {
		t.Fatalf("uncertainty not retained: %v", err)
	}
	if b.startCalls.Load() != 1 {
		t.Fatal("uncertain transfer retried")
	}
}

func TestCanceledStatusWaiterDoesNotAbandonQuery(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	snapshot := admit(t, c, b, "gid-1")
	ctx, cancel := context.WithCancel(t.Context())
	first := asyncStatus(c, ctx, snapshot.Ref)
	call := receive(t, b.statuses)
	cancel()
	if r := receive(t, first); !errors.Is(r.err, context.Canceled) {
		t.Fatalf("canceled status = %+v", r)
	}
	joined := observeWait(t.Context())
	second := asyncStatus(c, joined, snapshot.Ref)
	receive(t, joined.waiting)
	if call.ctx.Err() != nil {
		t.Fatal("waiter abandoned shared query")
	}
	call.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusActive, CompletedLength: 50}}
	if r := receive(t, second); r.err != nil || r.snapshot.Download.CompletedLength != 50 || b.statusCalls.Load() != 1 {
		t.Fatalf("shared query = %+v", r)
	}
}

func TestRejectedStartReleasesReservation(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	result := asyncStart(c, t.Context())
	call := receive(t, b.starts)
	rejected := errors.New("explicit rejection")
	call.reply <- backendResult{err: rejected}
	if r := receive(t, result); !errors.Is(r.err, rejected) {
		t.Fatalf("rejection = %+v", r)
	}
	admit(t, c, b, "gid-2")
	if b.startCalls.Load() != 2 || b.shutdownCalls.Load() != 0 {
		t.Fatal("rejection faulted admission")
	}
}

func TestBackendLossConfirmsTerminalFailure(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	snapshot := admit(t, c, b, "gid-1")
	result := asyncStatus(c, t.Context(), snapshot.Ref)
	call := receive(t, b.statuses)
	call.reply <- backendResult{err: download.ErrBackendLost}
	r := receive(t, result)
	if !errors.Is(r.err, download.ErrBackendLost) || !r.snapshot.Download.IsError() || r.snapshot.Download.ErrorMessage == "" || r.snapshot.Ref != snapshot.Ref || r.snapshot.Download.Filename != "file.zip" {
		t.Fatalf("backend loss = %+v", r)
	}
}

func TestCoordinatorParentCancellationClosesBackend(t *testing.T) {
	b := newCoordinatorBackend()
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	c := NewDownloadCoordinator(parent, b, "/downloads")
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := c.Close(cleanup); err != nil {
			t.Errorf("parent cleanup: %v", err)
		}
	})
	result := asyncStart(c, t.Context())
	call := receive(t, b.starts)
	cancel()
	receive(t, b.shutdown)
	if r := receive(t, result); !errors.Is(r.err, download.ErrClosed) {
		t.Fatalf("parent cancellation = %+v", r)
	}
	cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := c.Close(cleanup); err != nil || call.ctx.Err() == nil {
		t.Fatalf("parent cleanup: %v", err)
	}
}

func TestCoordinatorCloseDeadlineDoesNotAbandonCleanup(t *testing.T) {
	b := newCoordinatorBackend()
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	b.shutdownGate = gate
	c := testCoordinator(t, b)
	t.Cleanup(release)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("bounded close = %v", err)
	}
	receive(t, b.shutdown)
	joined := observeWait(t.Context())
	closed := make(chan error, 1)
	go func() { closed <- c.Close(joined) }()
	receive(t, joined.waiting)
	// Cleanup still owns the backend after the first waiter gives up.
	if b.shutdownCalls.Load() != 1 {
		t.Fatal("multiple shutdown operations")
	}
	release()
	if err := receive(t, closed); err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorBackendShutdownHasIndependentBound(t *testing.T) {
	for _, parentCloses := range []bool{false, true} {
		t.Run(map[bool]string{false: "caller", true: "parent"}[parentCloses], func(t *testing.T) {
			parent, stop := context.WithCancel(t.Context())
			defer stop()
			observed := make(chan context.Context, 1)
			b := &closingBackend{shutdown: func(ctx context.Context) error {
				if err := ctx.Err(); err != nil {
					t.Errorf("shutdown inherited root cancellation: %v", err)
				}
				observed <- ctx
				return nil
			}}
			c := NewDownloadCoordinator(parent, b, t.TempDir())
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if parentCloses {
				stop()
			} else if err := c.Close(ctx); err != nil {
				t.Fatal(err)
			}
			backendCtx := receive(t, observed)
			deadline, ok := backendCtx.Deadline()
			if !ok || deadline.After(time.Now().Add(2*time.Second)) {
				t.Fatalf("backend shutdown is unbounded: %v %v", deadline, ok)
			}
			if !parentCloses {
				callerDeadline, _ := ctx.Deadline()
				if deadline.After(callerDeadline) {
					t.Fatalf("backend exceeded close deadline: %v > %v", deadline, callerDeadline)
				}
			}
			if err := c.Close(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCoordinatorCloseRetriesWaitingForBackendDrain(t *testing.T) {
	var calls atomic.Int64
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	b := &closingBackend{shutdown: func(ctx context.Context) error {
		if calls.Add(1) == 1 {
			return context.DeadlineExceeded // Stop requested, but drain not complete.
		}
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	c := NewDownloadCoordinator(t.Context(), b, t.TempDir())
	if err := c.Close(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost backend cleanup timeout: %v", err)
	}
	result := make(chan error, 1)
	go func() { result <- c.Close(t.Context()) }()
	select {
	case <-entered:
	case err := <-result:
		t.Fatalf("repeated close did not await backend completion: %v", err)
	case <-time.After(time.Second):
		t.Fatal("repeated close did not reach backend drain")
	}
	unblock()
	if err := receive(t, result); err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorCloseForwardsEarlierDeadlineAfterParentStop(t *testing.T) {
	parent, stop := context.WithCancel(t.Context())
	defer stop()
	entered := make(chan context.Context, 2)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	var calls atomic.Int64
	b := &closingBackend{shutdown: func(ctx context.Context) error {
		entered <- ctx
		if calls.Add(1) == 1 {
			<-release
		} else {
			unblock() // Both waiters observe the same backend completion.
		}
		return nil
	}}
	c := NewDownloadCoordinator(parent, b, t.TempDir())
	stop()
	receive(t, entered) // Root shutdown has already admitted its owned stop worker.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("earlier close deadline never reached backend: %v", err)
	}
	forwarded := receive(t, entered)
	want, _ := ctx.Deadline()
	got, ok := forwarded.Deadline()
	if !ok || !got.Equal(want) {
		t.Fatalf("shared deadline was not forwarded: %v != %v", got, want)
	}
}

type checkedContext struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (ctx *checkedContext) Err() error {
	err := ctx.Context.Err()
	ctx.once.Do(func() { close(ctx.checked) })
	return err
}

func TestCanceledRequestBeforeReservationDoesNotStart(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	ctx, cancel := context.WithCancel(t.Context())
	checked := &checkedContext{Context: ctx, checked: make(chan struct{})}
	// Hold the admission boundary while cancellation overtakes the first check.
	c.mu.Lock()
	unlock := sync.OnceFunc(c.mu.Unlock)
	defer unlock()
	result := asyncStart(c, checked)
	receive(t, checked.checked)
	cancel()
	unlock()
	if r := receive(t, result); !errors.Is(r.err, context.Canceled) {
		t.Fatalf("canceled admission = %+v", r)
	}
	if err := c.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if b.startCalls.Load() != 0 {
		t.Fatal("cancellation before reservation started a transfer")
	}
}

func TestCanceledStatusBeforeQueryDoesNotStart(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	snapshot := admit(t, c, b, "gid-1")
	ctx, cancel := context.WithCancel(t.Context())
	checked := &checkedContext{Context: ctx, checked: make(chan struct{})}
	c.mu.Lock()
	unlock := sync.OnceFunc(c.mu.Unlock)
	defer unlock()
	result := asyncStatus(c, checked, snapshot.Ref)
	receive(t, checked.checked)
	cancel()
	unlock()
	if r := receive(t, result); !errors.Is(r.err, context.Canceled) {
		t.Fatalf("canceled query = %+v", r)
	}
	if err := c.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if b.statusCalls.Load() != 0 {
		t.Fatal("cancellation before query admission started a query")
	}
}
