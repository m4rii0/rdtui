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
