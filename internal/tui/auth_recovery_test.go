package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/pkg/models"
)

type recoveryOAuth struct{ starts, exchanges int }

func (o *recoveryOAuth) StartDeviceAuth(context.Context, string) (models.DeviceCode, error) {
	o.starts++
	return models.DeviceCode{DeviceCode: fmt.Sprint(o.starts), Interval: 1, ExpiresIn: 600, RequestedAt: time.Now()}, nil
}
func (*recoveryOAuth) PollDeviceCredentials(context.Context, string, string) (string, string, error) {
	return "client", "secret", nil
}
func (o *recoveryOAuth) ExchangeDeviceToken(context.Context, string, string, string) (models.DeviceCredentials, error) {
	o.exchanges++
	return models.DeviceCredentials{ClientID: "client", ClientSecret: "secret", AccessToken: "device", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (*recoveryOAuth) RefreshToken(context.Context, string, string, string) (models.DeviceCredentials, error) {
	panic("unexpected refresh")
}

type recoveryStore struct{ err error }

func (*recoveryStore) LoadDeviceCredentials() (*models.DeviceCredentials, error) {
	return nil, os.ErrNotExist
}
func (s *recoveryStore) SaveDeviceCredentials(models.DeviceCredentials) error { return s.err }

type recoveryPreferences struct{}

func (recoveryPreferences) EffectivePrivateToken() string { return "" }
func (recoveryPreferences) SavePrivateToken(string) error { return nil }

func TestInteractiveSaveRecoveryRetainsDeviceRetry(t *testing.T) {
	for _, retryKey := range []rune{'r', tea.KeyEnter} {
		t.Run(fmt.Sprint(retryKey), func(t *testing.T) {
			runtime := newTestRuntime(t)
			oauth := &recoveryOAuth{}
			store := &recoveryStore{err: os.ErrPermission}
			validations := 0
			manager := auth.NewManager(auth.SessionDependencies{Root: runtime.Context(), Spawn: runtime.Spawn,
				OAuth: oauth, Credentials: store, Preferences: recoveryPreferences{},
				Validate: func(context.Context, string) (models.User, error) {
					validations++
					return models.User{Username: "device"}, nil
				}})
			m := NewModel(Dependencies{Runtime: runtime, Auth: manager})
			m.mode = modeAuthChoice
			pressAndDeliver := func(key rune) {
				t.Helper()
				updated, cmd := m.Update(tea.KeyPressMsg{Code: key})
				m = updated.(Model)
				if cmd == nil {
					t.Fatalf("no command for %q in %s", key, m.mode)
				}
				updated, _ = m.Update(cmd())
				m = updated.(Model)
			}
			pressAndDeliver('2')
			code := *m.deviceCode
			pressAndDeliver('r')
			if m.mode != modeDeviceAuth || m.deviceCode == nil || *m.deviceCode != code || !strings.Contains(m.errText, "not been saved") {
				t.Fatalf("lost save recovery: mode=%s code=%+v error=%q", m.mode, m.deviceCode, m.errText)
			}
			if token, err := manager.Token(t.Context()); token != "" || !errors.Is(err, os.ErrPermission) || validations != 0 {
				t.Fatalf("pending credentials usable: token=%q err=%v validations=%d", token, err, validations)
			}
			// Even repeated retries with the disk still broken are save-only.
			pressAndDeliver(retryKey)
			store.err = nil
			pressAndDeliver(retryKey)
			if m.mode != modeMain || m.session == nil || m.deviceCode != nil || m.errText != "" {
				t.Fatalf("recovery did not authenticate: %s %+v %q", m.mode, m.session, m.errText)
			}
			if oauth.starts != 1 || oauth.exchanges != 1 || validations != 1 {
				t.Fatalf("recovery repeated OAuth: starts=%d exchanges=%d validations=%d", oauth.starts, oauth.exchanges, validations)
			}
		})
	}
}

type startupRecoveryStore struct {
	creds models.DeviceCredentials
	err   error
}

func (s *startupRecoveryStore) LoadDeviceCredentials() (*models.DeviceCredentials, error) {
	return new(s.creds), nil
}

func (s *startupRecoveryStore) SaveDeviceCredentials(creds models.DeviceCredentials) error {
	if s.err != nil {
		return s.err
	}
	s.creds = creds
	return nil
}

type startupRecoveryOAuth struct {
	recoveryOAuth
	now       time.Time
	refreshes int
}

func (o *startupRecoveryOAuth) RefreshToken(context.Context, string, string, string) (models.DeviceCredentials, error) {
	o.refreshes++
	return models.DeviceCredentials{ClientID: "client", ClientSecret: "secret", AccessToken: "rotated-access", RefreshToken: "rotated-refresh", ExpiresAt: o.now.Add(time.Hour)}, nil
}

func TestStartupSaveRecoveryRetriesWithoutOAuth(t *testing.T) {
	for _, key := range []rune{'r', tea.KeyEnter} {
		t.Run(fmt.Sprint(key), func(t *testing.T) {
			runtime := newTestRuntime(t)
			now := time.Date(2030, time.October, 16, 12, 0, 0, 0, time.UTC)
			oauth := &startupRecoveryOAuth{now: now}
			store := &startupRecoveryStore{err: os.ErrPermission, creds: models.DeviceCredentials{
				ClientID: "client", ClientSecret: "secret", AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: now.Add(-time.Hour),
			}}
			validations := 0
			manager := auth.NewManager(auth.SessionDependencies{Root: runtime.Context(), Spawn: runtime.Spawn,
				OAuth: oauth, Credentials: store, Preferences: recoveryPreferences{}, Now: func() time.Time { return now },
				Validate: func(_ context.Context, token string) (models.User, error) {
					validations++
					if store.creds.AccessToken != token || store.creds.RefreshToken != "rotated-refresh" {
						t.Error("credentials used before pending save succeeded")
					}
					return models.User{Username: "device"}, nil
				}})
			m := NewModel(Dependencies{Runtime: runtime, Auth: manager})
			updated, _ := m.Update(bootstrapCmd(runtime.Context(), runtime, manager)())
			m = updated.(Model)
			if m.mode != modeAuthChoice || !strings.Contains(m.errText, "not been saved") {
				t.Fatalf("startup failure hidden: %s %q", m.mode, m.errText)
			}
			retry := func() {
				t.Helper()
				updated, cmd := m.Update(tea.KeyPressMsg{Code: key})
				m = updated.(Model)
				if cmd == nil {
					t.Fatal("startup persistence has no save-only retry")
				}
				_, duplicate := m.Update(tea.KeyPressMsg{Code: key})
				if duplicate != nil {
					t.Fatal("overlapping bootstrap retries admitted")
				}
				updated, _ = m.Update(cmd())
				m = updated.(Model)
			}
			retry() // Storage remains broken; retain both pending credentials and retry.
			if validations != 0 || !strings.Contains(renderAuthChoice(m), "Retry saving credentials") {
				t.Fatal("pending credentials used or recovery action not visible")
			}
			store.err = nil
			retry()
			if m.mode != modeMain || m.session == nil || m.errText != "" || manager.Snapshot().Method != models.AuthMethodDevice {
				t.Fatalf("save retry did not recover: %s %+v %q", m.mode, m.session, m.errText)
			}
			if oauth.refreshes != 1 || oauth.starts != 0 || oauth.exchanges != 0 || validations != 1 || store.creds.RefreshToken != "rotated-refresh" {
				t.Fatalf("repeated OAuth/lost rotation: refreshes=%d starts=%d exchanges=%d validations=%d", oauth.refreshes, oauth.starts, oauth.exchanges, validations)
			}
			if strings.Contains(renderAuthChoice(m), "Retry saving credentials") {
				t.Fatal("successful recovery left a stale retry action")
			}
		})
	}
}
