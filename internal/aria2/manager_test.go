package aria2

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	m.dial = func(string, string) (rpcClient, error) {
		attempts++
		if attempts < 3 {
			return &fakeRPCClient{versionErr: errors.New("not ready")}, nil
		}
		return ready, nil
	}
	m.startupPollInterval = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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
	m.dial = func(string, string) (rpcClient, error) {
		return nil, errors.New("dial failed")
	}
	m.startupPollInterval = time.Millisecond
	waitDone := make(chan error, 1)
	waitDone <- errors.New("bind failed")
	close(waitDone)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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
	go m.watchProcess(proc, waitDone)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	err := m.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want %v", err, context.DeadlineExceeded)
	}
	if !proc.killed {
		t.Fatal("expected process kill fallback")
	}
	if !rpc.shutdownCalled {
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
	shutdownCalled bool
	closed         bool
}

func (f *fakeRPCClient) AddURI([]string, *arigo.Options) (arigo.GID, error) {
	return arigo.GID{GID: "gid"}, nil
}

func (f *fakeRPCClient) TellStatus(string, ...string) (arigo.Status, error) {
	return arigo.Status{}, nil
}

func (f *fakeRPCClient) GetVersion() (arigo.VersionInfo, error) {
	if f.versionErr != nil {
		return arigo.VersionInfo{}, f.versionErr
	}
	return arigo.VersionInfo{Version: "1.37.0"}, nil
}

func (f *fakeRPCClient) Shutdown() error {
	f.shutdownCalled = true
	return f.shutdownErr
}

func (f *fakeRPCClient) Close() error {
	f.closed = true
	return nil
}

type fakeProcess struct {
	waitCh chan error
	killed bool
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
	f.killed = true
	f.waitCh <- nil
	close(f.waitCh)
	return nil
}
