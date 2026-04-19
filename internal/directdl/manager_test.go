package directdl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	started, err := m.StartDownload(context.Background(), models.ManagedDownloadRequest{
		URL:      server.URL,
		Dir:      dir,
		Filename: "file.txt",
	})
	if err != nil {
		t.Fatalf("StartDownload() error = %v", err)
	}

	var status models.ManagedDownload
	for range 100 {
		status, err = m.DownloadStatus(context.Background(), started.GID)
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
