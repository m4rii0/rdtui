package app

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/config"
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

func TestServiceBulkQueueSharesSingleDownloadAdmission(t *testing.T) {
	svc := serviceWithBackend(t, &fakeDownloadManager{startDownload: func(context.Context, models.ManagedDownloadRequest) (models.ManagedDownload, error) {
		return models.ManagedDownload{GID: "single", Status: models.ManagedDownloadStatusActive}, nil
	}})
	started, err := svc.Start(t.Context(), "https://example.com/single", "single")
	if err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(t.Context())
	t.Cleanup(func() {
		r.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := r.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
	_, err = NewBulkQueue(QueueDependencies{Runtime: r, Downloads: svc}).Start(t.Context(), BulkPlan{})
	if !errors.Is(err, ErrActiveDownload) {
		t.Fatalf("queue did not reject unrelated transfer: %v", err)
	}
	current, ok := svc.Current()
	if !ok || current.Ref != started.Snapshot.Ref {
		t.Fatal("cannot reopen unrelated transfer")
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
	results := NewBatchService(nil, nil, nil).Import(ctx, []string{"unstarted-a.torrent", "unstarted-b.torrent"}).Items
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

	started, err := svc.Start(t.Context(), "https://example.com/file.zip", "")
	if err != nil {
		t.Fatalf("StartManagedDownload() error = %v", err)
	}
	if started.Snapshot.Download.GID != "gid-1" {
		t.Fatalf("gid = %q", started.Snapshot.Download.GID)
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
	if _, err := svc.Start(t.Context(), "https://example.com/first.zip", "first.zip"); err != nil {
		t.Fatal(err)
	}

	started, err := svc.Start(t.Context(), "https://example.com/other.zip", "other.zip")
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
	if _, err := svc.Start(t.Context(), "https://example.com/first.zip", ""); err != nil {
		t.Fatal(err)
	}
	// The single-download view was left, so no status polling observed completion.
	second, err := svc.Start(t.Context(), "https://example.com/second.zip", "")
	if err != nil || second.Reused || second.Snapshot.Download.GID != "second.zip" || second.Snapshot.Download.URL != "https://example.com/second.zip" || second.Snapshot.Download.Filename != "second.zip" || fake.startCalls.Load() != 2 {
		t.Fatalf("unpolled completion reopened the previous transfer: %+v, %v, starts=%d", second, err, fake.startCalls.Load())
	}
}

func TestIdentityStartRefreshesUnobservedCompletion(t *testing.T) {
	fake := &fakeDownloadManager{
		startDownload: func(_ context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: req.Filename, Status: models.ManagedDownloadStatusActive}, nil
		},
		downloadStatus: func(_ context.Context, gid string) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: gid, Status: models.ManagedDownloadStatusComplete}, nil
		},
	}
	svc := serviceWithBackend(t, fake)
	first, err := svc.Start(t.Context(), "https://example.com/first.zip", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Start(t.Context(), "https://example.com/second.zip", "")
	if err != nil || second.Reused || second.Snapshot.Ref.Generation <= first.Snapshot.Ref.Generation || second.Snapshot.Ref.GID != "second.zip" {
		t.Fatalf("reopened unobserved completion: %+v, %v", second, err)
	}
	if _, err := svc.Status(t.Context(), first.Snapshot.Ref); !errors.Is(err, ErrStaleDownload) {
		t.Fatalf("old reference accepted: %v", err)
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
	if _, err := svc.Start(t.Context(), "https://example.com/file.zip", "file.zip"); err != nil {
		t.Fatal(err)
	}

	current, ok := svc.Current()
	snapshot, err := svc.Status(t.Context(), current.Ref)
	status := snapshot.Download
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
	first, err := svc.Start(t.Context(), "https://example.com/file.zip", "")
	if err != nil {
		t.Fatal(err)
	}
	current, ok := svc.Current()
	snapshot, err := svc.Status(t.Context(), current.Ref)
	status := snapshot.Download
	if !errors.Is(err, transient) || !ok || status != first.Snapshot.Download {
		t.Fatalf("query error changed active download: %+v, %v, %v", status, ok, err)
	}
	second, err := svc.Start(t.Context(), "https://example.com/other", "")
	if err != nil || !second.Reused || second.Snapshot.Download.GID != first.Snapshot.Download.GID || fake.startCalls.Load() != 1 {
		t.Fatalf("query failure admitted a second download: %+v, %v", second, err)
	}
}

func TestExplicitTokenAuthenticationPreservesPreferences(t *testing.T) {
	f := newCompositionFixture(t)
	f.options.Overrides.PrivateToken = "environment-token"
	svc := newTestApplication(t, f.options)
	// Authentication and persistence must use the composition-time endpoints.
	t.Setenv("RDTUI_API_BASE_URL", "http://invalid.invalid")
	t.Setenv("RDTUI_PRIVATE_TOKEN", "later-environment-token")
	session, err := svc.Auth.AuthenticateWithToken(t.Context(), "entered-token", true)
	if err != nil || session.Token != "" || session.User.Username != "test-user" {
		t.Fatalf("explicit authentication = %+v, %v", session, err)
	}
	store := f.options.Store
	persisted, err := store.LoadPreferences()
	if err != nil || persisted != (config.Config{PrivateToken: "entered-token"}) {
		t.Fatalf("auth persisted effective values: %+v, %v", persisted, err)
	}
	if err := svc.Config.SavePrivateToken("second-entered-token"); err != nil {
		t.Fatal(err)
	}
	persisted, err = store.LoadPreferences()
	if err != nil || persisted != (config.Config{PrivateToken: "second-entered-token"}) {
		t.Fatalf("facade persisted effective values: %+v, %v", persisted, err)
	}
	if svc.Config.Effective().PrivateToken != "environment-token" || svc.Config.Effective().APIBaseURL != f.server.URL {
		t.Fatal("save changed the effective environment snapshot")
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

func serviceWithBackend(t *testing.T, backend download.Backend) *DownloadCoordinator {
	t.Helper()
	c := NewDownloadCoordinator(t.Context(), backend, "/tmp/downloads")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.Close(ctx); err != nil {
			t.Errorf("coordinator cleanup: %v", err)
		}
	})
	return c
}
