package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

type sessionOAuth struct {
	refresh       func(context.Context, string, string, string) (models.DeviceCredentials, error)
	exchange      func(context.Context, string, string, string) (models.DeviceCredentials, error)
	refreshCalls  atomic.Int32
	exchangeCalls atomic.Int32
}

func (o *sessionOAuth) StartDeviceAuth(context.Context, string) (models.DeviceCode, error) {
	return models.DeviceCode{DeviceCode: "device-code"}, nil
}
func (o *sessionOAuth) PollDeviceCredentials(context.Context, string, string) (string, string, error) {
	return "client", "secret", nil
}
func (o *sessionOAuth) ExchangeDeviceToken(ctx context.Context, id, secret, code string) (models.DeviceCredentials, error) {
	o.exchangeCalls.Add(1)
	return o.exchange(ctx, id, secret, code)
}
func (o *sessionOAuth) RefreshToken(ctx context.Context, id, secret, token string) (models.DeviceCredentials, error) {
	o.refreshCalls.Add(1)
	return o.refresh(ctx, id, secret, token)
}

type sessionStore struct {
	mu      sync.Mutex
	creds   *models.DeviceCredentials
	loadErr error
	saveErr error
	saves   int
}

func (s *sessionStore) LoadDeviceCredentials() (*models.DeviceCredentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creds == nil {
		return nil, s.loadErr
	}
	return new(*s.creds), s.loadErr
}
func (s *sessionStore) SaveDeviceCredentials(c models.DeviceCredentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.creds = new(c)
	return nil
}

type sessionPreferences struct {
	token   string
	saveErr error
}

func (p *sessionPreferences) EffectivePrivateToken() string { return p.token }
func (p *sessionPreferences) SavePrivateToken(token string) error {
	if p.saveErr != nil {
		return p.saveErr
	}
	p.token = token
	return nil
}

func deviceCredentials(now time.Time, remaining time.Duration) models.DeviceCredentials {
	return models.DeviceCredentials{ClientID: "client", ClientSecret: "secret", AccessToken: "device", RefreshToken: "refresh", TokenType: "Bearer", CreatedAt: now, ExpiresAt: now.Add(remaining)}
}

func sessionDependencies(t *testing.T, store CredentialStore, oauth OAuthClient) SessionDependencies {
	t.Helper()
	root, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		select {
		case <-done:
		case <-cleanup.Done():
			t.Error("session workers failed to stop")
		}
	})
	return SessionDependencies{
		Root: root,
		Spawn: func(ctx context.Context, fn func(context.Context)) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			wg.Go(func() { fn(ctx) })
			return nil
		},
		OAuth: oauth, Credentials: store, Preferences: &sessionPreferences{}, Now: time.Now,
		Validate: func(_ context.Context, token string) (models.User, error) { return models.User{Username: token}, nil },
	}
}

func TestRefreshAtOneMinuteBoundary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		remaining time.Duration
		want      string
		refreshes int32
	}{
		{"61 seconds", 61 * time.Second, "device", 0}, {"60 seconds", time.Minute, "rotated", 1}, {"expired", -time.Second, "rotated", 1}, {"missing access", time.Hour, "rotated", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			creds := deviceCredentials(now, tc.remaining)
			if tc.name == "missing access" {
				creds.AccessToken = ""
			}
			store := &sessionStore{creds: &creds}
			oauth := &sessionOAuth{refresh: func(_ context.Context, id, secret, token string) (models.DeviceCredentials, error) {
				if id != "client" || secret != "secret" || token != "refresh" {
					t.Error("incorrect refresh credentials")
				}
				c := deviceCredentials(now, time.Hour)
				c.AccessToken = "rotated"
				return c, nil
			}}
			deps := sessionDependencies(t, store, oauth)
			deps.Now = func() time.Time { return now }
			m := NewManager(deps)
			if _, err := m.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			if token, err := m.Token(t.Context()); err != nil || token != tc.want {
				t.Fatalf("Token = %q, %v", token, err)
			}
			if oauth.refreshCalls.Load() != tc.refreshes {
				t.Fatal("wrong refresh boundary")
			}
			if m.Snapshot().Token != "" {
				t.Fatal("snapshot exposed bearer token")
			}
		})
	}
}

func TestConcurrentRefreshCoalesced(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		c := deviceCredentials(now, time.Hour)
		release := make(chan struct{})
		o := &sessionOAuth{refresh: func(ctx context.Context, _, _, _ string) (models.DeviceCredentials, error) {
			select {
			case <-release:
				c := deviceCredentials(now, time.Hour)
				c.AccessToken = "rotated"
				return c, nil
			case <-ctx.Done():
				return models.DeviceCredentials{}, ctx.Err()
			}
		}}
		d := sessionDependencies(t, &sessionStore{creds: &c}, o)
		d.Now = func() time.Time { return now }
		m := NewManager(d)
		if _, err := m.Bootstrap(t.Context()); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Hour)
		results := make(chan error, 20)
		for range 20 {
			go func() {
				token, err := m.Token(t.Context())
				if err == nil && token != "rotated" {
					err = fmt.Errorf("wrong token %q", token)
				}
				results <- err
			}()
		}
		synctest.Wait()
		if o.refreshCalls.Load() != 1 {
			t.Fatal("duplicate refresh")
		}
		close(release)
		for range 20 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestRefreshWaiterCanCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := deviceCredentials(time.Now(), -time.Second)
		release := make(chan struct{})
		o := &sessionOAuth{refresh: func(ctx context.Context, _, _, _ string) (models.DeviceCredentials, error) {
			select {
			case <-release:
				c := deviceCredentials(time.Now(), time.Hour)
				c.AccessToken = "rotated"
				return c, nil
			case <-ctx.Done():
				return models.DeviceCredentials{}, ctx.Err()
			}
		}}
		m := NewManager(sessionDependencies(t, &sessionStore{creds: &c}, o))
		ctx, cancel := context.WithCancel(t.Context())
		first := make(chan error, 1)
		go func() { _, err := m.Bootstrap(ctx); first <- err }()
		synctest.Wait()
		second := make(chan error, 1)
		go func() { _, err := m.Token(t.Context()); second <- err }()
		synctest.Wait()
		cancel()
		if err := <-first; !errors.Is(err, context.Canceled) {
			t.Fatalf("waiter error %v", err)
		}
		if o.refreshCalls.Load() != 1 {
			t.Fatal("duplicate refresh")
		}
		close(release)
		if err := <-second; err != nil {
			t.Fatal(err)
		}
	})
}

func TestRefreshBoundedByRootAndTimeout(t *testing.T) {
	for _, cause := range []string{"root", "timeout"} {
		t.Run(cause, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c := deviceCredentials(time.Now(), -time.Second)
				o := &sessionOAuth{refresh: func(ctx context.Context, _, _, _ string) (models.DeviceCredentials, error) {
					<-ctx.Done()
					return models.DeviceCredentials{}, ctx.Err()
				}}
				d := sessionDependencies(t, &sessionStore{creds: &c}, o)
				root, cancel := context.WithCancel(d.Root)
				defer cancel()
				d.Root = root
				m := NewManager(d)
				result := make(chan error, 1)
				go func() { _, err := m.Bootstrap(t.Context()); result <- err }()
				synctest.Wait()
				want := context.Canceled
				if cause == "root" {
					cancel()
				} else {
					time.Sleep(29 * time.Second)
					select {
					case err := <-result:
						t.Fatalf("refresh ended early: %v", err)
					default:
					}
					time.Sleep(time.Second)
					want = context.DeadlineExceeded
				}
				if err := <-result; !errors.Is(err, want) {
					t.Fatalf("refresh error %v, want %v", err, want)
				}
			})
		})
	}
}

func TestOldRefreshCannotReplaceNewSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := deviceCredentials(time.Now(), -time.Second)
		store := &sessionStore{creds: &c}
		release := make(chan struct{})
		o := &sessionOAuth{refresh: func(context.Context, string, string, string) (models.DeviceCredentials, error) {
			<-release
			c := deviceCredentials(time.Now(), time.Hour)
			c.AccessToken = "stale"
			return c, nil
		}}
		m := NewManager(sessionDependencies(t, store, o))
		done := make(chan struct{})
		go func() { defer close(done); _, _ = m.Bootstrap(t.Context()) }()
		synctest.Wait()
		if _, err := m.AuthenticateWithToken(t.Context(), "new-user", false); err != nil {
			t.Fatal(err)
		}
		close(release)
		<-done
		if m.Snapshot().User.Username != "new-user" {
			t.Fatal("old session overwrote new")
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		if store.saves != 0 {
			t.Fatal("stale refresh saved over new generation")
		}
	})
}

func TestOldRefreshCannotSaveOverNewDeviceSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := deviceCredentials(time.Now(), -time.Second)
		store := &sessionStore{creds: &c}
		release := make(chan struct{})
		o := &sessionOAuth{
			refresh: func(context.Context, string, string, string) (models.DeviceCredentials, error) {
				<-release
				c := deviceCredentials(time.Now(), time.Hour)
				c.AccessToken = "stale"
				return c, nil
			},
			exchange: func(context.Context, string, string, string) (models.DeviceCredentials, error) {
				c := deviceCredentials(time.Now(), time.Hour)
				c.AccessToken = "new-device"
				c.RefreshToken = "new-refresh"
				return c, nil
			},
		}
		m := NewManager(sessionDependencies(t, store, o))
		done := make(chan struct{})
		go func() { defer close(done); _, _ = m.Bootstrap(t.Context()) }()
		synctest.Wait()
		if _, err := m.CompleteDeviceFlow(t.Context(), models.DeviceCode{DeviceCode: "new-code"}); err != nil {
			t.Fatal(err)
		}
		close(release)
		<-done
		if m.Snapshot().User.Username != "new-device" {
			t.Fatal("old session overwrote new device user")
		}
		stored, err := store.LoadDeviceCredentials()
		if err != nil || stored.AccessToken != "new-device" || stored.RefreshToken != "new-refresh" {
			t.Fatalf("old refresh overwrote durable device credentials: %+v %v", stored, err)
		}
	})
}

func TestPendingRotatedCredentialsRetrySave(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprint(expired), func(t *testing.T) {
			now := time.Now()
			c := deviceCredentials(now, -time.Second)
			diskErr := errors.New("disk full")
			store := &sessionStore{creds: &c, saveErr: diskErr}
			o := &sessionOAuth{refresh: func(_ context.Context, _, _, token string) (models.DeviceCredentials, error) {
				c := deviceCredentials(now, time.Hour)
				c.AccessToken = "rotated"
				c.RefreshToken = "rotated-refresh"
				if token == "rotated-refresh" {
					c.AccessToken = "renewed"
				} else if token != "refresh" {
					t.Error("lost refresh credential")
				}
				return c, nil
			}}
			d := sessionDependencies(t, store, o)
			d.Now = func() time.Time { return now }
			var validations atomic.Int32
			d.Validate = func(_ context.Context, token string) (models.User, error) {
				validations.Add(1)
				return models.User{Username: token}, nil
			}
			m := NewManager(d)
			_, err := m.Bootstrap(t.Context())
			if _, ok := errors.AsType[*CredentialPersistenceError](err); !ok || !errors.Is(err, diskErr) {
				t.Fatalf("persistence error = %v", err)
			}
			if token, err := m.Token(t.Context()); token != "" || !errors.Is(err, diskErr) {
				t.Fatalf("pending token exposed: %q, %v", token, err)
			}
			if validations.Load() != 0 {
				t.Fatal("pending credentials reached validation API")
			}
			if o.refreshCalls.Load() != 1 {
				t.Fatal("refreshed again before saving rotated credentials")
			}
			store.mu.Lock()
			store.saveErr = nil
			store.mu.Unlock()
			want := "rotated"
			calls := int32(1)
			if expired {
				now = now.Add(2 * time.Hour)
				want = "renewed"
				calls = 2
			}
			if token, err := m.Token(t.Context()); err != nil || token != want {
				t.Fatalf("recovery = %q, %v", token, err)
			}
			if o.refreshCalls.Load() != calls {
				t.Fatal("rotated token lost")
			}
		})
	}
}

func TestInteractiveDeviceCredentialsPendingSave(t *testing.T) {
	store := &sessionStore{saveErr: os.ErrPermission}
	var validations atomic.Int32
	o := &sessionOAuth{exchange: func(_ context.Context, id, secret, code string) (models.DeviceCredentials, error) {
		if id != "client" || secret != "secret" || code != "device-code" {
			t.Error("wrong device exchange")
		}
		return deviceCredentials(time.Now(), time.Hour), nil
	}}
	d := sessionDependencies(t, store, o)
	d.Validate = func(context.Context, string) (models.User, error) {
		validations.Add(1)
		return models.User{Username: "device-user"}, nil
	}
	m := NewManager(d)
	code, err := m.StartDeviceFlow(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CompleteDeviceFlow(t.Context(), code); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("completion error %v", err)
	}
	if _, err := m.CompleteDeviceFlow(t.Context(), code); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("pending completion retry error %v", err)
	}
	if o.exchangeCalls.Load() != 1 {
		t.Fatal("pending completion repeated the OAuth exchange")
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); _, _ = w.Write([]byte(`{"id":1}`)) }))
	defer server.Close()
	client := realdebrid.NewClient("", server.URL, server.URL, realdebrid.WithTokenSource(m.Token))
	if _, err := client.User(t.Context()); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("API error %v", err)
	}
	if requests.Load() != 0 || validations.Load() != 0 {
		t.Fatal("pending token reached API")
	}
	store.mu.Lock()
	store.saveErr = nil
	store.mu.Unlock()
	if _, err := m.CompleteDeviceFlow(t.Context(), code); err != nil {
		t.Fatalf("completion save recovery: %v", err)
	}
	if token, err := m.Token(t.Context()); err != nil || token != "device" {
		t.Fatalf("recovery %q %v", token, err)
	}
	if o.exchangeCalls.Load() != 1 || o.refreshCalls.Load() != 0 {
		t.Fatal("retried OAuth instead of save")
	}
}

func TestInteractiveDeviceCanceledCompletionCannotCommit(t *testing.T) {
	for _, cause := range []string{"caller replaced by new session", "root shutdown"} {
		t.Run(cause, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store := &sessionStore{}
				entered := make(chan struct{})
				release := make(chan struct{})
				oauth := &sessionOAuth{exchange: func(context.Context, string, string, string) (models.DeviceCredentials, error) {
					close(entered)
					<-release // Model a successful OAuth result arriving after cancellation.
					return deviceCredentials(time.Now(), time.Hour), nil
				}}
				deps := sessionDependencies(t, store, oauth)
				root, shutdown := context.WithCancel(deps.Root)
				defer shutdown()
				deps.Root = root
				m := NewManager(deps)
				if _, err := m.AuthenticateWithToken(t.Context(), "existing-user", false); err != nil {
					t.Fatal(err)
				}
				caller, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					_, err := m.CompleteDeviceFlow(caller, models.DeviceCode{DeviceCode: "canceled-code"})
					done <- err
				}()
				<-entered
				wantUser := "existing-user"
				if cause == "root shutdown" {
					shutdown()
				} else {
					cancel()
					if _, err := m.AuthenticateWithToken(t.Context(), "new-user", false); err != nil {
						t.Fatal(err)
					}
					wantUser = "new-user"
				}
				close(release)
				if err := <-done; !errors.Is(err, context.Canceled) {
					t.Errorf("completion error = %v, want cancellation", err)
				}
				if snapshot := m.Snapshot(); snapshot.Method != models.AuthMethodToken || snapshot.User.Username != wantUser {
					t.Errorf("canceled completion overwrote active session: %+v", snapshot)
				}
				store.mu.Lock()
				defer store.mu.Unlock()
				if store.saves != 0 || store.creds != nil {
					t.Errorf("canceled completion persisted credentials: %d saves", store.saves)
				}
			})
		})
	}
}

func TestMissingCredentialsDifferFromCorruptOrUnreadable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		missing bool
	}{
		{"missing", os.ErrNotExist, true}, {"empty", nil, true}, {"corrupt", errors.New("malformed JSON"), false}, {"unreadable", os.ErrPermission, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(sessionDependencies(t, &sessionStore{loadErr: tc.err}, &sessionOAuth{}))
			_, err := m.Bootstrap(t.Context())
			if tc.missing {
				if !errors.Is(err, ErrNoValidCredentials) {
					t.Fatalf("missing error %v", err)
				}
				return
			}
			if _, ok := errors.AsType[*CredentialLoadError](err); !ok || !errors.Is(err, tc.err) || errors.Is(err, ErrNoValidCredentials) {
				t.Fatalf("load cause lost: %v", err)
			}
		})
	}
}

func TestPrivateTokenPrecedenceAndNoRefresh(t *testing.T) {
	p := &sessionPreferences{token: "explicit"}
	o := &sessionOAuth{}
	d := sessionDependencies(t, &sessionStore{loadErr: os.ErrPermission}, o)
	d.Preferences = p
	m := NewManager(d)
	if _, err := m.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if token, err := m.Token(t.Context()); token != "explicit" || err != nil {
		t.Fatalf("token %q %v", token, err)
	}
	if _, err := m.AuthenticateWithToken(t.Context(), "saved", true); err != nil {
		t.Fatal(err)
	}
	if p.token != "saved" || o.refreshCalls.Load() != 0 {
		t.Fatal("private token not persisted or was refreshed")
	}
	p.saveErr = os.ErrPermission
	if _, err := m.AuthenticateWithToken(t.Context(), "unsaved", true); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("save error %v", err)
	}
	if token, _ := m.Token(t.Context()); token != "saved" {
		t.Fatal("failed save published token")
	}
	validationErr := errors.New("explicit private token rejected")
	d.Validate = func(context.Context, string) (models.User, error) { return models.User{}, validationErr }
	invalid := NewManager(d)
	if _, err := invalid.Bootstrap(t.Context()); !errors.Is(err, validationErr) {
		t.Fatalf("invalid explicit token fell back to device credentials: %v", err)
	}
}

func TestRejectOldTokenPreservesNewSession(t *testing.T) {
	m := NewManager(sessionDependencies(t, &sessionStore{}, &sessionOAuth{}))
	if _, err := m.AuthenticateWithToken(t.Context(), "old", false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AuthenticateWithToken(t.Context(), "new", false); err != nil {
		t.Fatal(err)
	}
	if m.RejectToken("old") || m.RejectToken("") {
		t.Fatal("old rejection invalidated new session")
	}
	if token, err := m.Token(t.Context()); token != "new" || err != nil {
		t.Fatalf("new token %q %v", token, err)
	}
	if !m.RejectToken("new") {
		t.Fatal("active token not rejected")
	}
	if token, err := m.Token(t.Context()); token != "" || !errors.Is(err, ErrNoValidCredentials) {
		t.Fatalf("rejected token available: %q %v", token, err)
	}
}
