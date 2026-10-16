package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/m4rii0/rdtui/pkg/models"
)

type OAuthClient interface {
	StartDeviceAuth(context.Context, string) (models.DeviceCode, error)
	PollDeviceCredentials(context.Context, string, string) (string, string, error)
	ExchangeDeviceToken(context.Context, string, string, string) (models.DeviceCredentials, error)
	RefreshToken(context.Context, string, string, string) (models.DeviceCredentials, error)
}

type CredentialStore interface {
	LoadDeviceCredentials() (*models.DeviceCredentials, error)
	SaveDeviceCredentials(models.DeviceCredentials) error
}

type TokenPreferences interface {
	EffectivePrivateToken() string
	SavePrivateToken(string) error
}

type SessionDependencies struct {
	Root        context.Context
	Spawn       func(context.Context, func(context.Context)) error
	OAuth       OAuthClient
	Validate    func(context.Context, string) (models.User, error)
	Credentials CredentialStore
	Preferences TokenPreferences
	Now         func() time.Time
}

type CredentialPersistenceError struct{ Err error }

func (e *CredentialPersistenceError) Error() string {
	return fmt.Sprintf("credentials have not been saved; reauthentication may be needed after exit: %v", e.Err)
}
func (e *CredentialPersistenceError) Unwrap() error { return e.Err }

type CredentialLoadError struct{ Err error }

func (e *CredentialLoadError) Error() string {
	return fmt.Sprintf("load device credentials: %v", e.Err)
}
func (e *CredentialLoadError) Unwrap() error { return e.Err }

var errSessionReplaced = errors.New("session replaced")

type refreshCall struct {
	done chan struct{}
	err  error
}

// Manager owns credentials, not just a token copied into a request client.
// mu also serializes durable commits with generation replacement. OAuth and
// validation run outside it; their results must still belong to this generation.
type Manager struct {
	mu                sync.Mutex
	deps              SessionDependencies
	generation        uint64
	session           models.AuthSession
	creds             *models.DeviceCredentials
	pending           *models.DeviceCredentials
	pendingDeviceCode string
	refresh           *refreshCall
}

func NewManager(deps SessionDependencies) *Manager {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Manager{deps: deps, session: models.AuthSession{Method: models.AuthMethodNone}}
}

// Snapshot is display-only: callers must obtain bearer tokens through Token.
func (m *Manager) Snapshot() models.AuthSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := m.session
	snapshot.Token = ""
	return snapshot
}

// RejectToken attributes a confirmed rejection to the token actually sent.
func (m *Manager) RejectToken(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if token == "" || m.session.Token != token {
		return false
	}
	m.replaceLocked()
	return true
}

func (m *Manager) replaceLocked() {
	m.generation++
	m.session = models.AuthSession{Method: models.AuthMethodNone}
	m.creds, m.pending, m.refresh = nil, nil, nil
	m.pendingDeviceCode = ""
}

func (m *Manager) needsRefresh(creds models.DeviceCredentials) bool {
	return creds.AccessToken == "" || !creds.ExpiresAt.After(m.deps.Now().Add(time.Minute))
}

func canRefresh(creds models.DeviceCredentials) bool {
	return creds.ClientID != "" && creds.ClientSecret != "" && creds.RefreshToken != ""
}

// Token coalesces work per generation, but each caller owns only its wait.
// Spawn registers the shared worker with the application runtime, never with a
// caller's context. Even if all waiters leave, the root and timeout bound it.
func (m *Manager) Token(ctx context.Context) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := m.deps.Root.Err(); err != nil {
			return "", err
		}
		m.mu.Lock()
		if m.pending == nil && m.session.Token != "" && (m.creds == nil || !m.needsRefresh(*m.creds)) {
			token := m.session.Token
			m.mu.Unlock()
			return token, nil
		}
		if m.creds == nil && m.pending == nil {
			m.mu.Unlock()
			return "", ErrNoValidCredentials
		}
		call := m.refresh
		if call == nil {
			call = &refreshCall{done: make(chan struct{})}
			m.refresh = call
			generation := m.generation
			err := m.deps.Spawn(m.deps.Root, func(root context.Context) {
				work, cancel := context.WithTimeout(root, 30*time.Second)
				defer cancel()
				err := m.refreshDevice(work, generation)
				m.mu.Lock()
				call.err = err
				if m.refresh == call {
					m.refresh = nil
				}
				close(call.done)
				m.mu.Unlock()
			})
			if err != nil {
				m.refresh = nil
				call.err = err
				close(call.done)
			}
		}
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-m.deps.Root.Done():
			return "", m.deps.Root.Err()
		case <-call.done:
			if call.err != nil && !errors.Is(call.err, errSessionReplaced) {
				return "", call.err
			}
		}
	}
}

// savePendingLocked never discards a rotated refresh token on a failed write.
func (m *Manager) savePendingLocked() error {
	if m.pending == nil {
		return nil
	}
	if err := m.deps.Credentials.SaveDeviceCredentials(*m.pending); err != nil {
		return &CredentialPersistenceError{Err: err}
	}
	m.creds, m.pending = m.pending, nil
	m.pendingDeviceCode = ""
	return nil
}

func (m *Manager) refreshDevice(ctx context.Context, generation uint64) error {
	m.mu.Lock()
	if m.generation != generation {
		m.mu.Unlock()
		return errSessionReplaced
	}
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return err
	}
	if err := m.savePendingLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	creds := *m.creds
	m.mu.Unlock()
	if m.needsRefresh(creds) {
		if !canRefresh(creds) {
			return ErrNoValidCredentials
		}
		refreshed, err := m.deps.OAuth.RefreshToken(ctx, creds.ClientID, creds.ClientSecret, creds.RefreshToken)
		if err != nil {
			return fmt.Errorf("refresh device token: %w", err)
		}
		m.mu.Lock()
		if m.generation != generation {
			m.mu.Unlock()
			return errSessionReplaced
		}
		m.pending = new(refreshed)
		m.session.Token = ""
		if err := ctx.Err(); err != nil {
			m.mu.Unlock()
			return err
		}
		err = m.savePendingLocked()
		m.mu.Unlock()
		if err != nil {
			return err
		}
		creds = refreshed
		// A malformed/short-lived OAuth result must not cause an unbounded loop.
		if m.needsRefresh(creds) {
			return ErrNoValidCredentials
		}
	}
	user, err := m.deps.Validate(ctx, creds.AccessToken)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generation != generation {
		return errSessionReplaced
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.session = models.AuthSession{Method: models.AuthMethodDevice, Token: creds.AccessToken, User: user}
	return nil
}
