package tui

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/directdl"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

func authApplication(t *testing.T, handler http.Handler, now func() time.Time) *app.Application {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	store := config.NewStoreAt(t.TempDir())
	if err := store.SaveDeviceCredentials(models.DeviceCredentials{ClientID: "client", ClientSecret: "secret", AccessToken: "device", RefreshToken: "refresh", ExpiresAt: now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	a, err := app.NewApplication(t.Context(), app.ApplicationOptions{Store: store, Home: t.TempDir(), HTTPClient: server.Client(), Now: now,
		Overrides: config.Overrides{APIBaseURL: server.URL, OAuthBaseURL: server.URL},
		Stat:      func(string) (int64, bool, error) { return 0, false, nil },
		BackendFactory: func(context.Context, config.EffectiveConfig) (download.Backend, error) {
			return directdl.NewManager(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := a.Close(cleanup); err != nil {
			t.Error(err)
		}
	})
	return a
}

func TestComposedRefreshRejectionRoutesToLogin(t *testing.T) {
	for _, stage := range []string{"refresh", "validation"} {
		t.Run(stage, func(t *testing.T) {
			now := time.Now()
			var refreshes, mutations atomic.Int32
			a := authApplication(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/user":
					if r.Header.Get("Authorization") != "Bearer rotated" {
						_, _ = io.WriteString(w, `{"username":"device"}`)
						return
					}
				case "/token":
					refreshes.Add(1)
					if stage == "validation" {
						_, _ = io.WriteString(w, `{"access_token":"rotated","refresh_token":"rotated-refresh","expires_in":3600}`)
						return
					}
				default:
					mutations.Add(1)
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error":"bad token","error_code":8}`)
			}), func() time.Time { return now })
			session, err := a.Auth.Bootstrap(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			m := NewModel(Dependencies{Runtime: a.Runtime, Auth: a.Auth, AuthState: a.Auth, Torrents: a.Torrents})
			m.session, m.mode = session, modeMain
			now = now.Add(time.Hour)
			_, err = a.Torrents.AddMagnet(t.Context(), "magnet:?xt=fixture")
			apiErr, ok := errors.AsType[*realdebrid.APIError](err)
			if !errors.Is(err, auth.ErrNoValidCredentials) || !ok || apiErr.StatusCode != 401 || apiErr.Code != 8 {
				t.Fatalf("lost composed rejection: %v", err)
			}
			updated, _ := m.Update(addTorrentMsg{err: err})
			m = updated.(Model)
			if m.mode != modeAuthChoice || m.session != nil || a.Auth.Snapshot().Method != models.AuthMethodNone {
				t.Fatalf("not routed to login: mode=%s session=%+v", m.mode, m.session)
			}
			if refreshes.Load() != 1 || mutations.Load() != 0 {
				t.Fatalf("request replayed/passed rejection: refresh=%d mutation=%d", refreshes.Load(), mutations.Load())
			}
		})
	}
}

func TestUnseenOldQueueRejectionUsesAuthoritativeAuthState(t *testing.T) {
	a := authApplication(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			_, _ = io.WriteString(w, `{"username":"fixture"}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"bad token","error_code":8}`)
	}), time.Now)
	session, err := a.Auth.Bootstrap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	m, queue := observedQueue(t)
	m.authService, m.authState, m.session = a.Auth, a.Auth, session
	_, firstErr := a.Torrents.ListTorrents(t.Context())
	snapshot := queue.snapshot
	snapshot.Items = slices.Clone(snapshot.Items)
	snapshot.CurrentIndex = 1
	snapshot.Items[0].Status = app.QueueItemTerminal
	snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeFailed, Err: firstErr}
	snapshot.Items[1].Status = app.QueueItemResolving
	m, _ = updateQueue(m, snapshot)
	if m.mode != modeAuthChoice || m.session != nil {
		t.Fatal("current rejection did not request login")
	}
	// The queue continues while login has stopped UI polling. Its second error
	// belongs to the no-credentials state before the replacement login.
	_, oldErr := a.Torrents.ListTorrents(t.Context())
	if !errors.Is(oldErr, auth.ErrNoValidCredentials) {
		t.Fatalf("missing old rejection: %v", oldErr)
	}
	snapshot.Items = slices.Clone(snapshot.Items)
	snapshot.Items[1].Status = app.QueueItemTerminal
	snapshot.Items[1].Outcome = &app.ItemOutcome{TorrentID: "b", Kind: app.OutcomeFailed, Err: oldErr}
	snapshot.Finished = true
	queue.snapshot = snapshot
	replacement, err := a.Auth.AuthenticateWithToken(t.Context(), "replacement", false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(authMsg{session: replacement})
	m = updated.(Model)
	updated, _ = m.handleDownloadAction()
	m = updated.(Model)
	updated, poll := m.handleShortcutAction(actionRefresh)
	m = updated.(Model)
	if poll == nil {
		t.Fatal("no queue poll after login")
	}
	updated, _ = m.Update(poll())
	m = updated.(Model)
	if m.mode != modeBulkDownload || m.session == nil || a.Auth.Snapshot().Method != models.AuthMethodToken {
		t.Fatal("unseen historical rejection invalidated replacement session")
	}
	if outcome := m.bulk.Snapshot.Items[1].Outcome; outcome == nil || !errors.Is(outcome.Err, oldErr) {
		t.Fatal("historical failure was erased")
	}
	// A real rejection of the replacement session is still actionable, even
	// when delivered through an unseen queue outcome rather than a list result.
	_, currentErr := a.Torrents.ListTorrents(t.Context())
	if !errors.Is(currentErr, auth.ErrNoValidCredentials) || a.Auth.Snapshot().Method != models.AuthMethodNone {
		t.Fatalf("replacement not authoritatively rejected: %v", currentErr)
	}
	m, queue = observedQueue(t)
	m.authState, m.session = a.Auth, replacement
	snapshot = queue.snapshot
	snapshot.Items = slices.Clone(snapshot.Items)
	snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeFailed, Err: currentErr}
	m, _ = updateQueue(m, snapshot)
	if m.mode != modeAuthChoice || m.session != nil {
		t.Fatal("real replacement-session rejection suppressed")
	}
}
