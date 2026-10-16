package auth

import (
	"context"
	"errors"
	"os"

	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

var ErrNoValidCredentials = errors.New("no valid credentials")

// Bootstrap selects the preference repository's effective token before device
// credentials. Explicit token validation errors must not fall back to OAuth.
func (m *Manager) Bootstrap(ctx context.Context) (*models.AuthSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.creds == nil && m.session.Token == "" && m.pending == nil {
		if token := m.deps.Preferences.EffectivePrivateToken(); token != "" {
			m.mu.Unlock()
			return m.AuthenticateWithToken(ctx, token, false)
		}
		creds, err := m.deps.Credentials.LoadDeviceCredentials()
		if err != nil {
			m.mu.Unlock()
			if errors.Is(err, os.ErrNotExist) {
				return nil, ErrNoValidCredentials
			}
			return nil, &CredentialLoadError{Err: err}
		}
		if creds == nil || (creds.AccessToken == "" && !canRefresh(*creds)) {
			m.mu.Unlock()
			return nil, ErrNoValidCredentials
		}
		m.replaceLocked()
		m.creds = new(*creds)
		m.session.Method = models.AuthMethodDevice
	}
	m.mu.Unlock()
	return m.deviceSession(ctx)
}

func (m *Manager) AuthenticateWithToken(ctx context.Context, token string, persist bool) (*models.AuthSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if token == "" {
		return nil, ErrNoValidCredentials
	}
	user, err := m.deps.Validate(ctx, token)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := m.deps.Root.Err(); err != nil {
		return nil, err
	}
	if persist {
		if err := m.deps.Preferences.SavePrivateToken(token); err != nil {
			return nil, &CredentialPersistenceError{Err: err}
		}
	}
	m.replaceLocked()
	m.session = models.AuthSession{Method: models.AuthMethodToken, Token: token, User: user}
	return new(m.session), nil
}

func (m *Manager) StartDeviceFlow(ctx context.Context) (models.DeviceCode, error) {
	return m.deps.OAuth.StartDeviceAuth(ctx, realdebrid.OpenSourceClientID)
}

func (m *Manager) CompleteDeviceFlow(ctx context.Context, code models.DeviceCode) (*models.AuthSession, error) {
	m.mu.Lock()
	retrySave := m.pending != nil && m.pendingDeviceCode == code.DeviceCode
	m.mu.Unlock()
	if retrySave {
		return m.deviceSession(ctx)
	}
	id, secret, err := m.deps.OAuth.PollDeviceCredentials(ctx, realdebrid.OpenSourceClientID, code.DeviceCode)
	if err != nil {
		return nil, err
	}
	creds, err := m.deps.OAuth.ExchangeDeviceToken(ctx, id, secret, code.DeviceCode)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	// Replacement and saving share the same boundary as a refresh commit. The
	// new credentials remain pending even when their first save fails.
	m.replaceLocked()
	m.session.Method = models.AuthMethodDevice
	m.pending = new(creds)
	m.pendingDeviceCode = code.DeviceCode
	err = m.savePendingLocked()
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return m.deviceSession(ctx)
}

func (m *Manager) deviceSession(ctx context.Context) (*models.AuthSession, error) {
	if _, err := m.Token(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return new(m.session), nil
}

func (m *Manager) ClearPrivateToken() error { return m.deps.Preferences.SavePrivateToken("") }
