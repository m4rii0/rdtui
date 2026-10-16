package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/aria2"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/directdl"
	"github.com/m4rii0/rdtui/pkg/models"
)

type failingWriter struct {
	err error
}

func TestEveryBackendEntryUsesDirectDefault(t *testing.T) {
	for _, backend := range []string{"", "invalid", "direct", "aria2"} {
		t.Run(backend, func(t *testing.T) {
			manager, err := productionBackend(t.Context(), config.EffectiveConfig{DownloadBackend: models.DownloadBackend(backend)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := manager.Shutdown(ctx); err != nil {
					t.Error(err)
				}
			})
			if backend == "aria2" {
				if _, ok := manager.(*aria2.Manager); !ok {
					t.Fatalf("explicit aria2 = %T", manager)
				}
			} else if _, ok := manager.(*directdl.Manager); !ok {
				t.Fatalf("backend %q = %T, want direct", backend, manager)
			}
		})
	}
}

func TestCompositionMapsApplicationComponents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer captured" {
			t.Errorf("request token: %s", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/user":
			_, _ = io.WriteString(w, `{"id":1,"username":"composed"}`)
		case "/torrents":
			_, _ = io.WriteString(w, `[{"id":"id","filename":"composed.iso"}]`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	for key, value := range map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "APPDATA": t.TempDir(), "HOME": t.TempDir(), "USERPROFILE": t.TempDir(),
		"RDTUI_API_BASE_URL": server.URL, "RDTUI_OAUTH_BASE_URL": server.URL, "RDTUI_PRIVATE_TOKEN": "captured"} {
		t.Setenv(key, value)
	}
	options, err := productionOptions()
	if err != nil {
		t.Fatal(err)
	}
	options.HTTPClient = server.Client()
	t.Setenv("RDTUI_PRIVATE_TOKEN", "later-token")
	t.Setenv("RDTUI_API_BASE_URL", "http://invalid.invalid")
	a, err := app.NewApplication(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanupRuntime(a.Runtime, a.Close); err != nil {
			t.Error(err)
		}
	})
	deps := applicationDependencies(a)
	if deps.Runtime != a.Runtime || deps.Auth != a.Auth || deps.Torrents != a.Torrents || deps.Downloads != a.Downloads || deps.Batches != a.Batches || deps.Bulk != a.Bulk || deps.Desktop != a.Desktop || deps.PrepareDownload == nil {
		t.Fatal("command replaced application dependencies")
	}
	session, err := deps.Auth.Bootstrap(t.Context())
	if err != nil || session.User.Username != "composed" || session.Token != "" {
		t.Fatalf("auth component: %+v %v", session, err)
	}
	torrents, err := deps.Torrents.ListTorrents(t.Context())
	if err != nil || len(torrents) != 1 || torrents[0].Filename != "composed.iso" {
		t.Fatalf("torrent component: %v %v", torrents, err)
	}
}

func TestRuntimeCleanupUsesIndependentBudget(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	runtime := app.NewRuntime(parent)
	cancel()
	want := errors.New("backend close failed")
	err := cleanupRuntime(runtime, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			t.Fatalf("cleanup inherited canceled root: %v", err)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Fatal("cleanup is not bounded")
		}
		if err := runtime.Do(t.Context(), func(context.Context) error { t.Error("admitted"); return nil }); !errors.Is(err, app.ErrClosed) {
			t.Fatal(err)
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
}

func TestRuntimeCleanupPreservesCloseAndDrainErrors(t *testing.T) {
	runtime := app.NewRuntime(t.Context())
	release := make(chan struct{})
	defer close(release)
	t.Cleanup(func() {
		runtime.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := runtime.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
	if err := runtime.Spawn(t.Context(), func(context.Context) { <-release }); err != nil {
		t.Fatal(err)
	}
	want := errors.New("backend close failed")
	err := cleanupRuntime(runtime, func(ctx context.Context) error {
		<-ctx.Done()
		return want
	})
	if !errors.Is(err, want) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost cleanup error: %v", err)
	}
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestPrintVersionPropagatesWriteError(t *testing.T) {
	want := errors.New("output unavailable")
	if err := printVersion(failingWriter{err: want}); !errors.Is(err, want) {
		t.Fatalf("printVersion error = %v, want %v", err, want)
	}
}

func TestRunReturnsErrorWhenVersionOutputFails(t *testing.T) {
	var stderr bytes.Buffer
	want := errors.New("output unavailable")
	if code := run([]string{"--version"}, failingWriter{err: want}, &stderr); code != 1 {
		t.Fatalf("run exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), want.Error()) {
		t.Fatalf("stderr = %q, want output error", stderr.String())
	}
}

func TestRunPreservesUsageExitCodeWhenStderrFails(t *testing.T) {
	var stdout bytes.Buffer
	stderr := failingWriter{err: errors.New("stderr unavailable")}
	if code := run([]string{"update", "--unknown"}, &stdout, stderr); code != 2 {
		t.Fatalf("run exit code = %d, want 2", code)
	}
}
