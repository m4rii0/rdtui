package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

var ErrNoValidCredentials = errors.New("no valid credentials")

type Manager struct {
	store  *config.Store
	prefs  *config.Repository
	cfg    config.EffectiveConfig
	client *realdebrid.Client
}

func NewManager(store *config.Store, prefs *config.Repository) *Manager {
	cfg := prefs.Effective()
	return &Manager{
		store:  store,
		prefs:  prefs,
		cfg:    cfg,
		client: realdebrid.NewClient("", cfg.APIBaseURL, cfg.OAuthBaseURL),
	}
}

func (m *Manager) Bootstrap(ctx context.Context) (*models.AuthSession, error) {
	if token := m.prefs.EffectivePrivateToken(); token != "" {
		return m.ValidateAndPersistToken(ctx, token, false)
	}

	creds, err := m.store.LoadDeviceCredentials()
	if err != nil {
		return nil, ErrNoValidCredentials
	}
	return m.sessionFromDeviceCredentials(ctx, creds)
}

func (m *Manager) ValidateAndPersistToken(ctx context.Context, token string, persist bool) (*models.AuthSession, error) {
	client := realdebrid.NewClient(token, m.cfg.APIBaseURL, m.cfg.OAuthBaseURL)
	user, err := client.User(ctx)
	if err != nil {
		return nil, err
	}
	if persist {
		if err := m.prefs.SavePrivateToken(token); err != nil {
			return nil, err
		}
	}
	return &models.AuthSession{Method: models.AuthMethodToken, Token: token, User: user}, nil
}

func (m *Manager) StartDeviceFlow(ctx context.Context) (models.DeviceCode, error) {
	return m.client.StartDeviceAuth(ctx, realdebrid.OpenSourceClientID)
}

func (m *Manager) CompleteDeviceFlow(ctx context.Context, code models.DeviceCode) (*models.AuthSession, error) {
	clientID, clientSecret, err := m.client.PollDeviceCredentials(ctx, realdebrid.OpenSourceClientID, code.DeviceCode)
	if err != nil {
		return nil, err
	}
	creds, err := m.client.ExchangeDeviceToken(ctx, clientID, clientSecret, code.DeviceCode)
	if err != nil {
		return nil, err
	}
	if err := m.store.SaveDeviceCredentials(creds); err != nil {
		return nil, err
	}
	return m.sessionFromDeviceCredentials(ctx, &creds)
}

func (m *Manager) sessionFromDeviceCredentials(ctx context.Context, creds *models.DeviceCredentials) (*models.AuthSession, error) {
	if creds == nil {
		return nil, ErrNoValidCredentials
	}
	if creds.AccessToken == "" || time.Now().After(creds.ExpiresAt.Add(-1*time.Minute)) {
		refreshed, err := m.client.RefreshToken(ctx, creds.ClientID, creds.ClientSecret, creds.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("refresh device token: %w", err)
		}
		creds = &refreshed
		if err := m.store.SaveDeviceCredentials(refreshed); err != nil {
			return nil, err
		}
	}
	client := realdebrid.NewClient(creds.AccessToken, m.cfg.APIBaseURL, m.cfg.OAuthBaseURL)
	user, err := client.User(ctx)
	if err != nil {
		return nil, err
	}
	return &models.AuthSession{Method: models.AuthMethodDevice, Token: creds.AccessToken, User: user}, nil
}

func (m *Manager) ClearPrivateToken() error {
	return m.prefs.SavePrivateToken("")
}
