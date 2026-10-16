package aria2

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/rpc2"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/pkg/models"
	"github.com/siku2/arigo"
)

func TestResolveBinaryPathMissing(t *testing.T) {
	t.Parallel()

	_, err := resolveBinaryPath("definitely-missing-aria2c-binary")
	if !errors.Is(err, ErrBinaryNotFound) {
		t.Fatalf("resolveBinaryPath() error = %v, want %v", err, ErrBinaryNotFound)
	}
}

func TestBuildCommandArgsIncludesManagedRPCFlags(t *testing.T) {
	t.Parallel()

	args := buildCommandArgs(8123, "secret-token")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--enable-rpc=true",
		"--rpc-listen-all=false",
		"--rpc-listen-port=8123",
		"--rpc-secret=secret-token",
		"--no-conf=true",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("buildCommandArgs() missing %q in %v", want, args)
		}
	}
}

func TestWaitForReadyRetriesUntilVersionSucceeds(t *testing.T) {
	t.Parallel()

	m := NewManager("")
	ready := &fakeRPCClient{}
	attempts := 0
	m.dial = func(context.Context, string, string) (rpcClient, error) {
		attempts++
		if attempts < 3 {
			return &fakeRPCClient{versionErr: errors.New("not ready")}, nil
		}
		return ready, nil
	}
	m.startupPollInterval = time.Millisecond

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	client, err := m.waitForReady(ctx, websocketURL(6800), "secret", make(chan error))
	if err != nil {
		t.Fatalf("waitForReady() error = %v", err)
	}
	if client != ready {
		t.Fatalf("waitForReady() client mismatch")
	}
	if attempts != 3 {
		t.Fatalf("dial attempts = %d, want 3", attempts)
	}
}

func TestWaitForReadyFailsWhenProcessExits(t *testing.T) {
	t.Parallel()

	m := NewManager("")
	m.dial = func(context.Context, string, string) (rpcClient, error) {
		return nil, errors.New("dial failed")
	}
	m.startupPollInterval = time.Millisecond
	waitDone := make(chan error, 1)
	waitDone <- errors.New("bind failed")
	close(waitDone)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	_, err := m.waitForReady(ctx, websocketURL(6800), "secret", waitDone)
	if err == nil || !strings.Contains(err.Error(), "bind failed") {
		t.Fatalf("waitForReady() error = %v, want bind failure", err)
	}
}

func TestShutdownKillsProcessAfterDeadline(t *testing.T) {
	t.Parallel()

	m := NewManager("")
	proc := newFakeProcess()
	rpc := &fakeRPCClient{}
	waitDone := make(chan error, 1)
	m.process = proc
	m.rpc = rpc
	m.waitDone = waitDone
	m.mu.Lock()
	m.wg.Go(func() { m.watchProcess(proc, waitDone) })
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()

	err := m.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want %v", err, context.DeadlineExceeded)
	}
	if !proc.killed.Load() {
		t.Fatal("expected process kill fallback")
	}
	if !rpc.shutdownCalled.Load() {
		t.Fatal("expected rpc shutdown before kill fallback")
	}
}

func TestMapStatusUsesDownloadFileMetadata(t *testing.T) {
	t.Parallel()

	status := arigo.Status{
		GID:             "gid-1",
		Status:          arigo.DownloadStatus("complete"),
		TotalLength:     2048,
		CompletedLength: 2048,
		DownloadSpeed:   512,
		Connections:     4,
		Dir:             "/tmp/downloads",
		Files: []arigo.File{{
			Path: "/tmp/downloads/movie.mkv",
		}},
	}

	download := mapStatus(status)
	if download.Status != "complete" {
		t.Fatalf("status = %q, want complete", download.Status)
	}
	if download.Filename != "movie.mkv" {
		t.Fatalf("filename = %q, want movie.mkv", download.Filename)
	}
	if download.FilePath != "/tmp/downloads/movie.mkv" {
		t.Fatalf("filePath = %q", download.FilePath)
	}
	if download.Directory != "/tmp/downloads" {
		t.Fatalf("directory = %q", download.Directory)
	}
}

type fakeRPCClient struct {
	versionErr     error
	shutdownErr    error
	shutdownCalled atomic.Bool
	closed         atomic.Bool
}

func (f *fakeRPCClient) AddURI([]string, *arigo.Options) (arigo.GID, error) {
	return arigo.GID{GID: "gid"}, nil
}

func (f *fakeRPCClient) TellStatus(string, ...string) (arigo.Status, error) {
	return arigo.Status{}, nil
}

func (f *fakeRPCClient) ForceRemove(string) error { return nil }

func (f *fakeRPCClient) GetVersion() (arigo.VersionInfo, error) {
	if f.versionErr != nil {
		return arigo.VersionInfo{}, f.versionErr
	}
	return arigo.VersionInfo{Version: "1.37.0"}, nil
}

func (f *fakeRPCClient) Shutdown() error {
	f.shutdownCalled.Store(true)
	return f.shutdownErr
}

func (f *fakeRPCClient) Close() error {
	f.closed.Store(true)
	return nil
}

type fakeProcess struct {
	waitCh chan error
	killed atomic.Bool
	once   sync.Once
}

func newFakeProcess() *fakeProcess {
	return &fakeProcess{waitCh: make(chan error, 1)}
}

func (f *fakeProcess) Start() error {
	return nil
}

func (f *fakeProcess) Wait() error {
	return <-f.waitCh
}

func (f *fakeProcess) Kill() error {
	f.killed.Store(true)
	f.once.Do(func() { f.waitCh <- nil; close(f.waitCh) })
	return nil
}

// Only the external RPC/process boundary is replaced: admission and worker
// ownership run through the real manager. Close interrupts blocked RPC calls.
type lifecycleRPC struct {
	closed    chan struct{}
	closeOnce sync.Once
	proc      *fakeProcess
	add       func() (arigo.GID, error)
	status    func(string) (arigo.Status, error)
	remove    func(string) error
	adds      atomic.Int64
	completed atomic.Int64
}

func (r *lifecycleRPC) AddURI([]string, *arigo.Options) (arigo.GID, error) {
	r.adds.Add(1)
	defer r.completed.Add(1)
	if r.add != nil {
		return r.add()
	}
	return arigo.GID{GID: "owned-gid"}, nil
}
func (r *lifecycleRPC) TellStatus(gid string, _ ...string) (arigo.Status, error) {
	if r.status != nil {
		return r.status(gid)
	}
	return arigo.Status{GID: gid, Status: "active"}, nil
}
func (r *lifecycleRPC) ForceRemove(gid string) error {
	if r.remove != nil {
		return r.remove(gid)
	}
	return nil
}
func (r *lifecycleRPC) GetVersion() (arigo.VersionInfo, error) {
	return arigo.VersionInfo{Version: "1"}, nil
}
func (r *lifecycleRPC) Shutdown() error { return r.proc.Kill() }
func (r *lifecycleRPC) Close() error    { r.closeOnce.Do(func() { close(r.closed) }); return nil }

func runningManager(t *testing.T, r *lifecycleRPC) *Manager {
	t.Helper()
	m := NewManager("definitely-missing-aria2c-binary")
	r.closed = make(chan struct{})
	r.proc = newFakeProcess()
	waitDone := make(chan error, 1)
	m.rpc, m.process, m.waitDone = r, r.proc, waitDone
	m.mu.Lock()
	m.wg.Go(func() { m.watchProcess(r.proc, waitDone) })
	m.mu.Unlock()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.Shutdown(ctx); err != nil {
			t.Errorf("cleanup shutdown: %v", err)
		}
	})
	return m
}

func expiredContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	t.Cleanup(cancel)
	return ctx
}

func blockedStart(t *testing.T) (*Manager, *lifecycleRPC, chan struct{}, <-chan error) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	r := &lifecycleRPC{}
	r.add = func() (arigo.GID, error) { close(entered); <-r.closed; <-release; return arigo.GID{}, io.EOF }
	m := runningManager(t, r)
	result := make(chan error, 1)
	go func() {
		_, err := m.StartDownload(t.Context(), models.ManagedDownloadRequest{URL: "https://example.com/file"})
		result <- err
	}()
	<-entered
	return m, r, release, result
}

func TestStartRacingWithShutdown(t *testing.T) {
	m, _, release, result := blockedStart(t)
	firstCloseErr := m.Shutdown(expiredContext(t))
	close(release)
	if !errors.Is(firstCloseErr, context.DeadlineExceeded) {
		t.Fatal(firstCloseErr)
	}
	_, startErr := m.StartDownload(t.Context(), models.ManagedDownloadRequest{})
	if !errors.Is(startErr, download.ErrClosed) {
		t.Fatalf("late start: %v", startErr)
	}
	<-result
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentShutdown(t *testing.T) {
	m, r, release, result := blockedStart(t)
	const callers = 8
	results := make(chan error, callers)
	ctx := expiredContext(t)
	for range callers {
		go func() { results <- m.Shutdown(ctx) }()
	}
	for range callers {
		if err := <-results; !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("shutdown did not wait for admitted start: %v", err)
		}
	}
	for range callers {
		go func() { results <- m.Shutdown(t.Context()) }()
	}
	close(release)
	for range callers {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	acceptedWorkers, completedWorkers := r.adds.Load(), r.completed.Load()
	if acceptedWorkers != completedWorkers {
		t.Fatal("shutdown left admitted work")
	}
	<-result
}

func TestShutdownDeadlineCanBeRetried(t *testing.T) {
	m, _, release, result := blockedStart(t)
	firstCloseErr := m.Shutdown(expiredContext(t))
	if !errors.Is(firstCloseErr, context.DeadlineExceeded) {
		close(release)
		t.Fatal(firstCloseErr)
	}
	_, startErr := m.StartDownload(t.Context(), models.ManagedDownloadRequest{})
	close(release)
	<-result
	secondCloseErr := m.Shutdown(t.Context())
	if secondCloseErr != nil {
		t.Fatal(secondCloseErr)
	}
	if !errors.Is(startErr, download.ErrClosed) {
		t.Fatalf("late start: %v", startErr)
	}
}

func TestTransferContextCancelsOwnedGID(t *testing.T) {
	removed := make(chan string, 1)
	confirmed := make(chan struct{})
	var canceled atomic.Bool
	r := &lifecycleRPC{
		remove: func(gid string) error { removed <- gid; canceled.Store(true); return nil },
		status: func(gid string) (arigo.Status, error) {
			if canceled.Load() {
				<-confirmed
				return arigo.Status{GID: gid, Status: "removed"}, nil
			}
			return arigo.Status{GID: gid, Status: "active"}, nil
		},
	}
	m := runningManager(t, r)
	ctx, cancel := context.WithCancel(t.Context())
	started, err := m.StartDownload(ctx, models.ManagedDownloadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case gid := <-removed:
		if gid != "owned-gid" || started.GID != gid {
			t.Fatalf("removed wrong transfer: %q, start=%+v", gid, started)
		}
	case <-time.After(time.Second):
		close(confirmed)
		t.Fatal("transfer cancellation did not remove owned GID")
	}
	// Cancellation is not complete while the backend still cannot confirm stop.
	err = m.Shutdown(expiredContext(t))
	close(confirmed)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown escaped cancellation confirmation: %v", err)
	}
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestAdmittedGIDSurvivesStatusError(t *testing.T) {
	r := &lifecycleRPC{status: func(string) (arigo.Status, error) { return arigo.Status{}, io.EOF }}
	m := runningManager(t, r)
	started, err := m.StartDownload(t.Context(), models.ManagedDownloadRequest{URL: "https://example.com/file", Filename: "file", Dir: "/downloads"})
	if err != nil || started.GID != "owned-gid" || started.Status != models.ManagedDownloadStatusWaiting || started.Filename != "file" || started.Directory != "/downloads" {
		t.Fatalf("admitted identity lost: %+v, %v", started, err)
	}
	_, err = m.DownloadStatus(t.Context(), started.GID)
	if err == nil || errors.Is(err, download.ErrBackendLost) {
		t.Fatalf("observation error confused with termination: %v", err)
	}
}

func TestUncertainAddURIClosesAdmission(t *testing.T) {
	r := &lifecycleRPC{add: func() (arigo.GID, error) { return arigo.GID{}, io.EOF }}
	m := runningManager(t, r)
	_, err := m.StartDownload(t.Context(), models.ManagedDownloadRequest{})
	if !errors.Is(err, download.ErrUncertainStart) {
		t.Fatalf("ambiguous start: %v", err)
	}
	_, err = m.StartDownload(t.Context(), models.ManagedDownloadRequest{})
	if !errors.Is(err, download.ErrClosed) || r.adds.Load() != 1 {
		t.Fatalf("uncertain start allowed duplicate: %v, adds=%d", err, r.adds.Load())
	}
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !r.proc.killed.Load() {
		t.Fatal("uncertain backend still running")
	}
}

func TestStartExplicitRejectionKeepsAdmission(t *testing.T) {
	r := &lifecycleRPC{add: func() (arigo.GID, error) { return arigo.GID{}, rpc2.ServerError("rejected") }}
	m := runningManager(t, r)
	for range 2 {
		_, err := m.StartDownload(t.Context(), models.ManagedDownloadRequest{})
		if err == nil || errors.Is(err, download.ErrClosed) || errors.Is(err, download.ErrUncertainStart) {
			t.Fatalf("explicit rejection: %v", err)
		}
	}
	if r.adds.Load() != 2 {
		t.Fatal("rejection closed admission")
	}
}

func TestStartShutdownInterruptsStartupDial(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(binary)
	proc := newFakeProcess()
	m.makeProcess = func(string, []string) (process, error) { return proc, nil }
	m.choosePort = func() (int, error) { return 6800, nil }
	m.makeSecret = func() (string, error) { return "secret", nil }
	entered, interrupted := make(chan struct{}), make(chan struct{})
	m.dial = func(ctx context.Context, _, _ string) (rpcClient, error) {
		close(entered)
		<-ctx.Done()
		close(interrupted)
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := m.StartDownload(ctx, models.ManagedDownloadRequest{}); result <- err }()
	<-entered
	if err := m.Shutdown(expiredContext(t)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	select {
	case <-interrupted:
	case <-time.After(time.Second):
		cancel()
		<-result
		_ = m.Shutdown(t.Context())
		t.Fatal("shutdown did not cancel startup dial")
	}
	<-result
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestTransferContextCanceledDuringAddIsUncertain(t *testing.T) {
	entered := make(chan struct{})
	r := &lifecycleRPC{}
	r.add = func() (arigo.GID, error) { close(entered); <-r.closed; return arigo.GID{}, io.EOF }
	m := runningManager(t, r)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { _, err := m.StartDownload(ctx, models.ManagedDownloadRequest{}); result <- err }()
	<-entered
	cancel()
	if err := <-result; !errors.Is(err, download.ErrUncertainStart) {
		t.Fatalf("canceled ambiguous add: %v", err)
	}
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestStartConfirmedProcessExitIsBackendLost(t *testing.T) {
	r := &lifecycleRPC{}
	m := runningManager(t, r)
	started, err := m.StartDownload(t.Context(), models.ManagedDownloadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	waitDone := m.waitDone
	m.mu.Unlock()
	_ = r.proc.Kill()
	<-waitDone
	_, err = m.DownloadStatus(t.Context(), started.GID)
	if !errors.Is(err, download.ErrBackendLost) {
		t.Fatalf("confirmed termination: %v", err)
	}
	_, err = m.StartDownload(t.Context(), models.ManagedDownloadRequest{})
	if !errors.Is(err, download.ErrBackendLost) {
		t.Fatalf("restarted lost backend: %v", err)
	}
}
