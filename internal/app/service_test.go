package app

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/aria2"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/directdl"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestDefaultFileSelectionChoosesLargestFile(t *testing.T) {
	info := models.TorrentInfo{Files: []models.TorrentFile{{ID: 1, Bytes: 100}, {ID: 2, Bytes: 200}, {ID: 3, Bytes: 150}}}
	got := DefaultFileSelection(info)
	want := []int{2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultFileSelection() = %v, want %v", got, want)
	}
}

func TestValidTorrentFilesFiltersAndDeduplicates(t *testing.T) {
	valid, invalid := ValidTorrentFiles([]string{"/tmp/a.torrent", "/tmp/a.torrent", "/tmp/b.txt", "/tmp/c.TORRENT"})
	if !reflect.DeepEqual(valid, []string{"/tmp/a.torrent", "/tmp/c.TORRENT"}) {
		t.Fatalf("valid = %v", valid)
	}
	if !reflect.DeepEqual(invalid, []string{"/tmp/b.txt"}) {
		t.Fatalf("invalid = %v", invalid)
	}
}

func TestImportTorrentFilesStopsBeforeReadingCanceledWork(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	svc := &Service{}
	results := svc.ImportTorrentFiles(ctx, []string{"unstarted-a.torrent", "unstarted-b.torrent"})
	if len(results) != 2 {
		t.Fatalf("results = %v", results)
	}
	for _, result := range results {
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("unstarted import = %+v", result)
		}
	}
}

func TestStartManagedDownloadUsesConfiguredDirAndFilename(t *testing.T) {
	t.Parallel()

	fake := &fakeDownloadManager{
		startDownload: func(_ context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
			if req.Dir != "/tmp/downloads" {
				t.Errorf("download dir = %q", req.Dir)
			}
			if req.Filename != "file.zip" {
				t.Errorf("filename = %q", req.Filename)
			}
			return models.ManagedDownload{GID: "gid-1"}, nil
		},
	}

	svc := serviceWithBackend(t, fake)

	started, err := svc.StartManagedDownload(t.Context(), "https://example.com/file.zip", "")
	if err != nil {
		t.Fatalf("StartManagedDownload() error = %v", err)
	}
	if started.Download.GID != "gid-1" {
		t.Fatalf("gid = %q", started.Download.GID)
	}
}

func TestStartManagedDownloadReusesActiveSession(t *testing.T) {
	t.Parallel()

	fake := &fakeDownloadManager{
		startDownload: func(context.Context, models.ManagedDownloadRequest) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: "gid-1", Status: models.ManagedDownloadStatusActive}, nil
		},
		downloadStatus: func(context.Context, string) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: "gid-1", Status: models.ManagedDownloadStatusActive}, nil
		},
	}

	svc := serviceWithBackend(t, fake)
	if _, err := svc.StartManagedDownload(t.Context(), "https://example.com/first.zip", "first.zip"); err != nil {
		t.Fatal(err)
	}

	started, err := svc.StartManagedDownload(t.Context(), "https://example.com/other.zip", "other.zip")
	if err != nil {
		t.Fatalf("StartManagedDownload() error = %v", err)
	}
	if !started.Reused {
		t.Fatal("expected active download reuse")
	}
	if fake.startCalls.Load() != 1 {
		t.Fatalf("startCalls = %d, want 1", fake.startCalls.Load())
	}
}

func TestStartManagedDownloadRefreshesCompletionWithoutPolling(t *testing.T) {
	fake := &fakeDownloadManager{
		startDownload: func(_ context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: req.Filename, Status: models.ManagedDownloadStatusActive}, nil
		},
		downloadStatus: func(_ context.Context, gid string) (models.ManagedDownload, error) {
			if gid != "first.zip" {
				t.Errorf("refreshed GID = %q, want first.zip", gid)
			}
			return models.ManagedDownload{GID: gid, Status: models.ManagedDownloadStatusComplete}, nil
		},
	}
	svc := serviceWithBackend(t, fake)
	if _, err := svc.StartManagedDownload(t.Context(), "https://example.com/first.zip", ""); err != nil {
		t.Fatal(err)
	}
	// The single-download view was left, so no status polling observed completion.
	second, err := svc.StartManagedDownload(t.Context(), "https://example.com/second.zip", "")
	if err != nil || second.Reused || second.Download.GID != "second.zip" || second.Download.URL != "https://example.com/second.zip" || second.Download.Filename != "second.zip" || fake.startCalls.Load() != 2 {
		t.Fatalf("unpolled completion reopened the previous transfer: %+v, %v, starts=%d", second, err, fake.startCalls.Load())
	}
}

func TestManagedDownloadStatusPreservesCachedMetadata(t *testing.T) {
	t.Parallel()

	fake := &fakeDownloadManager{
		startDownload: func(context.Context, models.ManagedDownloadRequest) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: "gid-1", Status: models.ManagedDownloadStatusActive}, nil
		},
		downloadStatus: func(context.Context, string) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: "gid-1", Status: models.ManagedDownloadStatusActive}, nil
		},
	}

	svc := serviceWithBackend(t, fake)
	if _, err := svc.StartManagedDownload(t.Context(), "https://example.com/file.zip", "file.zip"); err != nil {
		t.Fatal(err)
	}

	status, ok, err := svc.ManagedDownloadStatus(t.Context())
	if err != nil {
		t.Fatalf("ManagedDownloadStatus() error = %v", err)
	}
	if !ok {
		t.Fatal("expected active download")
	}
	if status.URL != "https://example.com/file.zip" || status.Filename != "file.zip" {
		t.Fatalf("preserved metadata = %+v", status)
	}
	if status.Directory != "/tmp/downloads" || status.FilePath != filepath.FromSlash("/tmp/downloads/file.zip") {
		t.Fatalf("preserved paths = %+v", status)
	}
}

func TestServiceStatusFailureKeepsActiveDownload(t *testing.T) {
	transient := errors.New("temporary query failure")
	fake := &fakeDownloadManager{
		startDownload: func(context.Context, models.ManagedDownloadRequest) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: "gid-1", Status: models.ManagedDownloadStatusActive}, nil
		},
		downloadStatus: func(context.Context, string) (models.ManagedDownload, error) {
			return models.ManagedDownload{}, transient
		},
	}
	svc := serviceWithBackend(t, fake)
	first, err := svc.StartManagedDownload(t.Context(), "https://example.com/file.zip", "")
	if err != nil {
		t.Fatal(err)
	}
	status, ok, err := svc.ManagedDownloadStatus(t.Context())
	if !errors.Is(err, transient) || !ok || status != first.Download {
		t.Fatalf("query error changed active download: %+v, %v, %v", status, ok, err)
	}
	second, err := svc.StartManagedDownload(t.Context(), "https://example.com/other", "")
	if err != nil || !second.Reused || second.Download.GID != first.Download.GID || fake.startCalls.Load() != 1 {
		t.Fatalf("query failure admitted a second download: %+v, %v", second, err)
	}
}

func TestNewDownloadManagerSelectsDirectBackend(t *testing.T) {
	t.Parallel()

	manager := newDownloadManager(config.Config{DownloadBackend: string(models.DownloadBackendDirect)})
	if _, ok := manager.(*directdl.Manager); !ok {
		t.Fatalf("manager = %T, want *directdl.Manager", manager)
	}

	manager = newDownloadManager(config.Config{DownloadBackend: string(models.DownloadBackendAria2)})
	if _, ok := manager.(*aria2.Manager); !ok {
		t.Fatalf("manager = %T, want *aria2.Manager", manager)
	}
}

type fakeDownloadManager struct {
	startCalls     atomic.Int64
	startDownload  func(context.Context, models.ManagedDownloadRequest) (models.ManagedDownload, error)
	downloadStatus func(context.Context, string) (models.ManagedDownload, error)
}

func (f *fakeDownloadManager) StartDownload(ctx context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
	f.startCalls.Add(1)
	if f.startDownload == nil {
		return models.ManagedDownload{}, nil
	}
	return f.startDownload(ctx, req)
}

func (f *fakeDownloadManager) DownloadStatus(ctx context.Context, gid string) (models.ManagedDownload, error) {
	if f.downloadStatus == nil {
		return models.ManagedDownload{}, nil
	}
	return f.downloadStatus(ctx, gid)
}

func (f *fakeDownloadManager) Shutdown(context.Context) error {
	return nil
}

func serviceWithBackend(t *testing.T, backend download.Backend) *Service {
	t.Helper()
	c := NewDownloadCoordinator(t.Context(), backend, "/tmp/downloads")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.Close(ctx); err != nil {
			t.Errorf("coordinator cleanup: %v", err)
		}
	})
	return &Service{config: config.Config{DefaultDownloadDir: "/tmp/downloads"}, downloads: c}
}
