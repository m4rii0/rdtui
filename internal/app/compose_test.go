package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type compositionFixture struct {
	options  ApplicationOptions
	server   *httptest.Server
	mu       sync.Mutex
	headers  map[string][]string
	reject   atomic.Bool
	external atomic.Int64
	clock    atomic.Int64
}

func newCompositionFixture(t *testing.T) *compositionFixture {
	t.Helper()
	f := &compositionFixture{headers: make(map[string][]string)}
	f.clock.Store(time.Date(2030, time.October, 16, 12, 0, 0, 0, time.UTC).Unix())
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Injected") != "yes" {
			t.Error("constructor bypassed injected transport")
		}
		f.mu.Lock()
		f.headers[r.URL.Path] = append(f.headers[r.URL.Path], r.Header.Get("Authorization"))
		f.mu.Unlock()
		if f.reject.Load() && r.URL.Path != "/user" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"bad_token","error_code":8}`)
			return
		}
		switch r.URL.Path {
		case "/user":
			_, _ = io.WriteString(w, `{"id":1,"username":"test-user"}`)
		case "/token":
			_, _ = io.WriteString(w, `{"access_token":"new-token","refresh_token":"rotated","expires_in":3600,"token_type":"Bearer"}`)
		case "/device/code":
			_, _ = io.WriteString(w, `{"device_code":"device","user_code":"code"}`)
		case "/torrents":
			_, _ = io.WriteString(w, `[]`)
		case "/torrents/info/id":
			_, _ = io.WriteString(w, `{"id":"id","filename":"file.iso"}`)
		case "/torrents/addMagnet", "/torrents/addTorrent":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"added"}`)
		case "/torrents/selectFiles/id", "/torrents/delete/id":
			w.WriteHeader(http.StatusNoContent)
		case "/unrestrict/link":
			_, _ = io.WriteString(w, `{"download":"https://download.invalid/file.iso","filename":"file.iso","filesize":42}`)
		case "/source":
			_, _ = io.WriteString(w, "torrent bytes")
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	transport := f.server.Client().Transport
	f.options = ApplicationOptions{
		Store: config.NewStoreAt(t.TempDir()), Home: t.TempDir(),
		Overrides: config.Overrides{APIBaseURL: f.server.URL, OAuthBaseURL: f.server.URL},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if !strings.HasPrefix(r.URL.String(), f.server.URL+"/") {
				f.external.Add(1)
				return nil, fmt.Errorf("external network denied: %s", r.URL)
			}
			r = r.Clone(r.Context())
			r.Header.Set("X-Injected", "yes")
			return transport.RoundTrip(r)
		})},
		Now:  func() time.Time { return time.Unix(f.clock.Load(), 0) },
		Stat: func(string) (int64, bool, error) { return 0, false, nil },
		BackendFactory: func(context.Context, config.EffectiveConfig) (download.Backend, error) {
			return &fakeDownloadManager{}, nil
		},
	}
	return f
}

func newTestApplication(t *testing.T, options ApplicationOptions) *Application {
	t.Helper()
	a, err := NewApplication(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := a.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return a
}

func TestCompositionUsesInjectedDependencies(t *testing.T) {
	f := newCompositionFixture(t)
	var factoryCalled bool
	f.options.BackendFactory = func(ctx context.Context, cfg config.EffectiveConfig) (download.Backend, error) {
		factoryCalled = true
		if ctx.Err() != nil || cfg.DefaultDownloadDir != filepath.Join(f.options.Home, "Downloads") || cfg.DownloadBackend != models.DownloadBackendDirect {
			t.Fatalf("factory config: %+v, %v", cfg, ctx.Err())
		}
		return &fakeDownloadManager{startDownload: func(_ context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
			if req.Dir != filepath.Join(f.options.Home, "Downloads") || req.Filename != "file.iso" {
				t.Errorf("backend request: %+v", req)
			}
			return models.ManagedDownload{GID: "injected", Status: models.ManagedDownloadStatusActive}, nil
		}}, nil
	}
	f.options.Stat = func(path string) (int64, bool, error) {
		if path != filepath.Join(f.options.Home, "Downloads", "file.iso") {
			t.Errorf("stat %s", path)
		}
		return 17, true, nil
	}
	var waits int
	clipboardErr := errors.New("injected clipboard unavailable")
	f.options.Desktop = failingCompositionDesktop{err: clipboardErr}
	f.options.Wait = func(ctx context.Context, d time.Duration) error {
		if d != 250*time.Millisecond {
			t.Errorf("delay %s", d)
		}
		waits++
		return ctx.Err()
	}
	a := newTestApplication(t, f.options)
	session, err := a.Auth.AuthenticateWithToken(t.Context(), "private", true)
	if err != nil || session.Token != "" || session.User.Username != "test-user" {
		t.Fatalf("display session: %+v, %v", session, err)
	}
	if code, err := a.Auth.StartDeviceFlow(t.Context()); err != nil || !code.RequestedAt.Equal(time.Unix(f.clock.Load(), 0)) {
		t.Fatalf("device clock: %+v %v", code, err)
	}
	prepared, err := a.PrepareDownload(t.Context(), models.DownloadTarget{Link: "link"})
	if err != nil || !prepared.Exists || prepared.ExistingBytes != 17 {
		t.Fatalf("preparation: %+v, %v", prepared, err)
	}
	result := a.Batches.Delete(t.Context(), []string{"id", "id"})
	if result.Err != nil || waits != 1 || !factoryCalled || f.external.Load() != 0 {
		t.Fatalf("wiring: %+v, waits=%d factory=%v external=%d", result, waits, factoryCalled, f.external.Load())
	}
	links := a.Batches.CopyLinks(t.Context(), []LinkRequest{{TorrentID: "id", Target: models.DownloadTarget{Link: "link"}, Eligible: true}})
	if !errors.Is(links.ClipboardErr, clipboardErr) || len(links.ResolvedURLs) != 1 || links.ResolvedURLs[0] != "https://download.invalid/file.iso" || links.CopiedURLs != 0 {
		t.Fatalf("desktop dependency lost: %+v", links)
	}
	if _, err := a.Downloads.Start(t.Context(), prepared.URL, prepared.Filename); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Bulk.Start(t.Context(), BulkPlan{}); !errors.Is(err, ErrActiveDownload) {
		t.Fatalf("queue has separate downloader: %v", err)
	}
	prefs, err := f.options.Store.LoadPreferences()
	if err != nil || prefs != (config.Config{PrivateToken: "private"}) {
		t.Fatalf("persisted effective config: %+v %v", prefs, err)
	}
}

type failingCompositionDesktop struct{ err error }

func (d failingCompositionDesktop) CopyURL(string) (bool, error)   { return false, d.err }
func (d failingCompositionDesktop) OpenFile(string) error          { return d.err }
func (d failingCompositionDesktop) RevealInDirectory(string) error { return d.err }

func authenticatedOperations(t *testing.T, a *Application, source string) map[string]func() error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file.torrent")
	if err := os.WriteFile(path, []byte("torrent"), 0600); err != nil {
		t.Fatal(err)
	}
	return map[string]func() error{
		"list":   func() error { _, err := a.Torrents.ListTorrents(t.Context()); return err },
		"detail": func() error { _, err := a.Torrents.TorrentInfo(t.Context(), "id"); return err },
		"magnet": func() error { _, err := a.Torrents.AddMagnet(t.Context(), "magnet"); return err },
		"url":    func() error { _, err := a.Torrents.AddTorrentURL(t.Context(), source); return err },
		"select": func() error { return a.Torrents.SelectFiles(t.Context(), "id", []int{1}) },
		"delete": func() error { return a.Torrents.DeleteTorrent(t.Context(), "id") },
		"resolve": func() error {
			_, err := a.PrepareDownload(t.Context(), models.DownloadTarget{Link: "link"})
			return err
		},
		"import": func() error { return a.Batches.Import(t.Context(), []string{path}).Items[0].Err },
	}
}

func TestAuthenticatedOperationsBeforeLoginReturnAuthRequired(t *testing.T) {
	f := newCompositionFixture(t)
	a := newTestApplication(t, f.options)
	for name, operation := range authenticatedOperations(t, a, f.server.URL+"/source") {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, auth.ErrNoValidCredentials) {
				t.Fatal(err)
			}
		})
	}
	if result := a.Batches.Import(t.Context(), []string{"does-not-exist.torrent"}); !errors.Is(result.Items[0].Err, auth.ErrNoValidCredentials) {
		t.Fatalf("import before authentication: %+v", result)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.headers) != 0 {
		t.Fatalf("unauthenticated operations performed I/O: %v", f.headers)
	}
}

func TestSessionRefreshReachesTorrentRequests(t *testing.T) {
	f := newCompositionFixture(t)
	now := time.Unix(f.clock.Load(), 0)
	if err := f.options.Store.SaveDeviceCredentials(models.DeviceCredentials{ClientID: "client", ClientSecret: "secret", AccessToken: "old-token", RefreshToken: "refresh", ExpiresAt: now.Add(2 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	a := newTestApplication(t, f.options)
	session, err := a.Auth.Bootstrap(t.Context())
	if err != nil || session.Token != "" {
		t.Fatalf("bootstrap: %+v %v", session, err)
	}
	if _, err := a.Torrents.ListTorrents(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.clock.Store(now.Add(90 * time.Second).Unix())
	for name, operation := range authenticatedOperations(t, a, f.server.URL+"/source") {
		t.Run(name, func(t *testing.T) {
			if err := operation(); err != nil {
				t.Fatal(err)
			}
		})
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for path, headers := range f.headers {
		if path == "/source" || path == "/token" || path == "/user" {
			continue
		}
		if headers[len(headers)-1] != "Bearer new-token" {
			t.Errorf("%s: %v", path, headers)
		}
	}
	if len(f.headers["/token"]) != 1 || f.external.Load() != 0 {
		t.Fatalf("refresh calls=%v external=%d", f.headers["/token"], f.external.Load())
	}
	if f.headers["/torrents"][0] != "Bearer old-token" || f.headers["/token"][0] != "" {
		t.Fatalf("incorrect static/OAuth token boundary: %v", f.headers)
	}
	creds, err := f.options.Store.LoadDeviceCredentials()
	if err != nil || creds.RefreshToken != "rotated" || !creds.ExpiresAt.Equal(now.Add(90*time.Second+time.Hour)) {
		t.Fatalf("credentials: %+v %v", creds, err)
	}
}

type closingBackend struct {
	fakeDownloadManager
	shutdown func(context.Context) error
}

func (b *closingBackend) Shutdown(ctx context.Context) error { return b.shutdown(ctx) }

func TestApplicationCloseCancelsBeforeDraining(t *testing.T) {
	f := newCompositionFixture(t)
	var root context.Context
	want := errors.New("backend shutdown")
	f.options.BackendFactory = func(ctx context.Context, _ config.EffectiveConfig) (download.Backend, error) {
		root = ctx
		return &closingBackend{shutdown: func(context.Context) error {
			if root.Err() == nil {
				t.Error("backend stopped before root cancellation")
			}
			return want
		}}, nil
	}
	a, err := NewApplication(t.Context(), f.options)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	releaseWorker := sync.OnceFunc(func() { close(release) })
	t.Cleanup(func() {
		releaseWorker()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := a.Close(ctx); !errors.Is(err, want) {
			t.Errorf("cleanup: %v", err)
		}
	})
	canceled := make(chan struct{})
	if err := a.Runtime.Spawn(t.Context(), func(ctx context.Context) { <-ctx.Done(); close(canceled); <-release }); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = a.Close(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("close did not bound drain: %v", err)
	}
	<-canceled
	if err := a.Runtime.Do(t.Context(), func(context.Context) error { t.Error("admitted after close"); return nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	releaseWorker()
	cleanup, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := a.Close(cleanup); !errors.Is(err, want) {
		t.Fatalf("lost shutdown error: %v", err)
	}
}

func TestCompositionRejectsOnlyCurrentToken(t *testing.T) {
	f := newCompositionFixture(t)
	a := newTestApplication(t, f.options)
	if _, err := a.Auth.AuthenticateWithToken(t.Context(), "current", false); err != nil {
		t.Fatal(err)
	}
	f.reject.Store(true)
	_, err := a.Torrents.ListTorrents(t.Context())
	apiErr, ok := errors.AsType[*realdebrid.APIError](err)
	if !errors.Is(err, auth.ErrNoValidCredentials) || !ok || apiErr.StatusCode != 401 {
		t.Fatalf("rejection lost causes: %v", err)
	}
	if _, err := a.Torrents.ListTorrents(t.Context()); !errors.Is(err, auth.ErrNoValidCredentials) {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.headers["/torrents"]) != 1 {
		t.Fatalf("replayed rejected request: %v", f.headers)
	}
}

func TestCompositionStaleRejectionPreservesNewSession(t *testing.T) {
	f := newCompositionFixture(t)
	transport := f.options.HTTPClient.Transport
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	f.options.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		response, err := transport.RoundTrip(r)
		if r.URL.Path == "/torrents" && r.Header.Get("Authorization") == "Bearer old" {
			close(entered)
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}
		return response, err
	})
	a := newTestApplication(t, f.options)
	if _, err := a.Auth.AuthenticateWithToken(t.Context(), "old", false); err != nil {
		t.Fatal(err)
	}
	f.reject.Store(true)
	result := make(chan error, 1)
	if err := a.Runtime.Spawn(t.Context(), func(ctx context.Context) { _, err := a.Torrents.ListTorrents(ctx); result <- err }); err != nil {
		t.Fatal(err)
	}
	<-entered
	if _, err := a.Auth.AuthenticateWithToken(t.Context(), "new", false); err != nil {
		t.Fatal(err)
	}
	// Release without closing so deferred cleanup remains safe on early failures.
	release <- struct{}{}
	err := <-result
	if errors.Is(err, auth.ErrNoValidCredentials) {
		t.Fatalf("stale rejection invalidated replacement: %v", err)
	}
	if _, ok := errors.AsType[*realdebrid.APIError](err); !ok {
		t.Fatalf("lost stale API error: %v", err)
	}
	f.reject.Store(false)
	if _, err := a.Torrents.ListTorrents(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := f.headers["/torrents"]; len(got) != 2 || got[0] != "Bearer old" || got[1] != "Bearer new" {
		t.Fatalf("request replay or lost replacement: %v", got)
	}
}
