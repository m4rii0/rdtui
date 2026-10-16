package directdl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
