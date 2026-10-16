package directdl

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestManagerDownloadsFileDirectly(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "12")
		_, _ = w.Write([]byte("hello world!"))
	}))
	defer server.Close()

	m := NewManager()
	t.Cleanup(func() {
		_ = m.Shutdown(context.Background())
	})

	dir := t.TempDir()
	started, err := m.StartDownload(t.Context(), models.ManagedDownloadRequest{
		URL:      server.URL,
		Dir:      dir,
		Filename: "file.txt",
	})
	if err != nil {
		t.Fatalf("StartDownload() error = %v", err)
	}

	var status models.ManagedDownload
	for range 100 {
		status, err = m.DownloadStatus(t.Context(), started.GID)
		if err != nil {
			t.Fatalf("DownloadStatus() error = %v", err)
		}
		if status.IsComplete() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !status.IsComplete() {
		t.Fatalf("status = %+v, want complete", status)
	}
	data, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "hello world!" {
		t.Fatalf("downloaded file = %q", data)
	}
}

// The barrier holds a start immediately before the admission/registration lock.
// Removing the closed check admits a worker after the shutdown waiter finished.
func TestStartRacingWithShutdown(t *testing.T) {
	m := NewManager()
	entered, release := make(chan struct{}), make(chan struct{})
	m.beforeAdmission = func() { close(entered); <-release }
	result := make(chan error, 1)
	dir := t.TempDir()
	go func() {
		_, err := m.StartDownload(t.Context(), models.ManagedDownloadRequest{Dir: dir, Filename: "file"})
		result <- err
	}()
	<-entered
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	close(release)
	if startErr := <-result; !errors.Is(startErr, download.ErrClosed) {
		t.Fatalf("late start: %v", startErr)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The transport acknowledges cancellation but cannot finish until released.
// This exposes premature shutdown completion and per-caller abandoned waiters.
func blockedDownload(t *testing.T) (*Manager, <-chan struct{}, chan struct{}, *atomic.Int64) {
	t.Helper()
	m := NewManager()
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	completed := new(atomic.Int64)
	var enteredOnce, canceledOnce sync.Once
	m.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		enteredOnce.Do(func() { close(entered) })
		<-r.Context().Done()
		canceledOnce.Do(func() { close(canceled) })
		<-release
		completed.Add(1)
		return nil, r.Context().Err()
	})
	_, err := m.StartDownload(t.Context(), models.ManagedDownloadRequest{URL: "https://example.com/file", Dir: t.TempDir(), Filename: "file"})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	return m, canceled, release, completed
}

func TestConcurrentShutdown(t *testing.T) {
	m, canceled, release, completed := blockedDownload(t)
	const callers = 8
	results := make(chan error, callers)
	for range callers {
		go func() { results <- m.Shutdown(t.Context()) }()
	}
	<-canceled
	select {
	case err := <-results:
		t.Fatalf("shutdown finished before worker: %v", err)
	default:
	}
	close(release)
	for range callers {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	acceptedWorkers, completedWorkers := int64(1), completed.Load()
	if acceptedWorkers != completedWorkers {
		t.Fatal("shutdown left admitted work")
	}
}

func TestShutdownDeadlineCanBeRetried(t *testing.T) {
	m, canceled, release, completed := blockedDownload(t)
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	firstCloseErr := m.Shutdown(ctx)
	if !errors.Is(firstCloseErr, context.DeadlineExceeded) {
		t.Fatal(firstCloseErr)
	}
	<-canceled
	_, startErr := m.StartDownload(t.Context(), models.ManagedDownloadRequest{Dir: t.TempDir(), Filename: "late"})
	close(release)
	secondCloseErr := m.Shutdown(t.Context())
	if secondCloseErr != nil {
		t.Fatal(secondCloseErr)
	}
	if !errors.Is(startErr, download.ErrClosed) {
		t.Fatalf("late start: %v", startErr)
	}
	if completed.Load() != 1 {
		t.Fatal("shutdown left admitted work")
	}
}

func TestTransferContextCancelsHTTP(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(t.Context())
	entered, canceled := make(chan struct{}), make(chan struct{})
	m.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		close(canceled)
		return nil, r.Context().Err()
	})
	_, err := m.StartDownload(ctx, models.ManagedDownloadRequest{URL: "https://example.com/file", Dir: t.TempDir(), Filename: "file"})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	cancel()
	// A deterministic barrier on the manager's task set, not scheduler timing.
	// Baseline has no transfer bridge, so inspect the actual request context.
	select {
	case <-canceled:
	case <-time.After(time.Second):
		_ = m.Shutdown(t.Context())
		t.Fatal("transfer cancellation did not reach HTTP request")
	}
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestManagerShutdownCancelsConcurrentDownloads(t *testing.T) {
	t.Parallel()
	const count = 8
	dir := t.TempDir()
	requests := make(chan struct{}, count)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	m := NewManager()
	t.Cleanup(func() {
		// t.Context is canceled before cleanup; shutdown needs its own context.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.Shutdown(ctx); err != nil {
			t.Errorf("cleanup Shutdown() error = %v", err)
		}
	})
	type result struct {
		download models.ManagedDownload
		err      error
	}
	results := make(chan result, count)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			download, err := m.StartDownload(ctx, models.ManagedDownloadRequest{
				URL: server.URL, Dir: dir, Filename: fmt.Sprintf("file-%d", i),
			})
			results <- result{download, err}
		})
	}
	wg.Wait()
	ids := make(map[string]bool, count)
	for range count {
		res := <-results
		if res.err != nil {
			t.Fatalf("StartDownload() error = %v", res.err)
		}
		if res.download.GID == "" || ids[res.download.GID] {
			t.Fatalf("download ID %q is empty or duplicated", res.download.GID)
		}
		ids[res.download.GID] = true
	}
	for range count {
		select {
		case <-requests:
		case <-ctx.Done():
			t.Fatalf("waiting for active requests: %v", ctx.Err())
		}
	}
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	for id := range ids {
		status, err := m.DownloadStatus(ctx, id)
		if err != nil {
			t.Fatalf("DownloadStatus(%q) error = %v", id, err)
		}
		if !status.IsError() || !strings.Contains(status.ErrorMessage, context.Canceled.Error()) {
			t.Errorf("download %q after shutdown = %+v, want cancellation error", id, status)
		}
	}
}
