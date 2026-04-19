package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/m4rii0/rdtui/internal/aria2"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/directdl"
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

func TestStartManagedDownloadUsesConfiguredDirAndFilename(t *testing.T) {
	t.Parallel()

	fake := &fakeDownloadManager{
		startDownload: func(_ context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
			if req.Dir != "/tmp/downloads" {
				t.Fatalf("download dir = %q", req.Dir)
			}
			if req.Filename != "file.zip" {
				t.Fatalf("filename = %q", req.Filename)
			}
			return models.ManagedDownload{GID: "gid-1"}, nil
		},
	}

	svc := &Service{config: config.Config{DefaultDownloadDir: "/tmp/downloads"}, downloader: fake}

	started, err := svc.StartManagedDownload(context.Background(), "https://example.com/file.zip", "")
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
		downloadStatus: func(context.Context, string) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: "gid-1", Status: models.ManagedDownloadStatusActive}, nil
		},
	}

	svc := &Service{
		downloader: fake,
		activeDownload: &models.ManagedDownload{
			GID:    "gid-1",
			Status: models.ManagedDownloadStatusActive,
		},
	}

	started, err := svc.StartManagedDownload(context.Background(), "https://example.com/other.zip", "other.zip")
	if err != nil {
		t.Fatalf("StartManagedDownload() error = %v", err)
	}
	if !started.Reused {
		t.Fatal("expected active download reuse")
	}
	if fake.startCalls != 0 {
		t.Fatalf("startCalls = %d, want 0", fake.startCalls)
	}
}

func TestManagedDownloadStatusPreservesCachedMetadata(t *testing.T) {
	t.Parallel()

	fake := &fakeDownloadManager{
		downloadStatus: func(context.Context, string) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: "gid-1", Status: models.ManagedDownloadStatusActive}, nil
		},
	}

	svc := &Service{
		downloader: fake,
		activeDownload: &models.ManagedDownload{
			GID:       "gid-1",
			URL:       "https://example.com/file.zip",
			Filename:  "file.zip",
			Directory: "/tmp/downloads",
			FilePath:  "/tmp/downloads/file.zip",
		},
	}

	status, ok, err := svc.ManagedDownloadStatus(context.Background())
	if err != nil {
		t.Fatalf("ManagedDownloadStatus() error = %v", err)
	}
	if !ok {
		t.Fatal("expected active download")
	}
	if status.URL != "https://example.com/file.zip" || status.Filename != "file.zip" {
		t.Fatalf("preserved metadata = %+v", status)
	}
	if status.Directory != "/tmp/downloads" || status.FilePath != "/tmp/downloads/file.zip" {
		t.Fatalf("preserved paths = %+v", status)
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
	startCalls     int
	startDownload  func(context.Context, models.ManagedDownloadRequest) (models.ManagedDownload, error)
	downloadStatus func(context.Context, string) (models.ManagedDownload, error)
}

func (f *fakeDownloadManager) SetBinaryPath(string) {}

func (f *fakeDownloadManager) StartDownload(ctx context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
	f.startCalls++
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
