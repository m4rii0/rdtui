package tui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

type AuthService interface {
	Bootstrap(context.Context) (*models.AuthSession, error)
	AuthenticateWithToken(context.Context, string, bool) (*models.AuthSession, error)
	StartDeviceFlow(context.Context) (models.DeviceCode, error)
	CompleteDeviceFlow(context.Context, models.DeviceCode) (*models.AuthSession, error)
}

// AuthState reads the session owner's current state, not the UI's older display
// snapshot. It authorizes navigation on delayed request/queue failures.
type AuthState interface {
	Snapshot() models.AuthSession
}

type authModel struct {
	authService AuthService
	authState   AuthState
	auth        asyncRequest
	session     *models.AuthSession
	deviceCode  *models.DeviceCode
}

// The root owns navigation/overlays; the feature owns authentication state and
// interprets its typed results before requesting a navigation transition.
func (a *authModel) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case bootstrapMsg:
		m.loading = false
		if msg.err == nil {
			a.session = msg.session
			m.mode = modeMain
			m.status = "Authenticated as " + msg.session.User.Username
			m.errText = ""
			return m.refresh()
		}
		m.mode = modeAuthChoice
		if errors.Is(msg.err, auth.ErrNoValidCredentials) {
			m.status = "Choose an authentication method"
			m.errText = ""
		} else {
			m.errText = msg.err.Error()
		}
	case authMsg:
		a.auth.invalidate()
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return nil
		}
		a.session = msg.session
		m.mode = modeMain
		m.input.Blur()
		m.input.Reset()
		m.status = "Authenticated as " + msg.session.User.Username
		m.errText = ""
		return m.refresh()
	case deviceStartMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			m.mode = modeAuthChoice
			return nil
		}
		a.deviceCode = new(msg.code)
		m.mode = modeDeviceAuth
		m.status = "Finish device authorization in your browser"
		m.errText = ""
		return m.after(time.Duration(max(1, msg.code.Interval))*time.Second, devicePollTickMsg{requestID{a.auth.Generation}})
	case devicePollMsg:
		if msg.err == nil {
			a.auth.invalidate()
			a.session = msg.session
			m.mode = modeMain
			a.deviceCode = nil
			m.status = "Authenticated as " + msg.session.User.Username
			m.errText = ""
			return m.refresh()
		}
		if _, ok := errors.AsType[*auth.CredentialPersistenceError](msg.err); ok {
			// Retain the flow identity: r/enter retries the manager's pending
			// save, not browser authorization or another OAuth exchange.
			m.errText = msg.err.Error()
			m.status = "Retry saving credentials with r or enter"
			return nil
		}
		if errors.Is(msg.err, realdebrid.ErrAuthorizationPending) {
			if a.deviceCode != nil && time.Since(a.deviceCode.RequestedAt) < time.Duration(a.deviceCode.ExpiresIn)*time.Second {
				return m.after(time.Duration(max(1, a.deviceCode.Interval))*time.Second, devicePollTickMsg{requestID{a.auth.Generation}})
			}
			m.errText = "device code expired"
		} else {
			m.errText = msg.err.Error()
		}
		m.mode = modeAuthChoice
		a.deviceCode = nil
	case devicePollTickMsg:
		if a.auth.current(msg.request) && !a.auth.InFlight && a.deviceCode != nil && m.mode == modeDeviceAuth {
			return m.pollDevice()
		}
	}
	return nil
}

func (m *Model) pollDevice() tea.Cmd {
	if m.auth.InFlight || m.deviceCode == nil {
		return nil
	}
	return m.beginFeature(authFeature, func(ctx context.Context) tea.Cmd {
		return pollDeviceCmd(ctx, m.runtime, m.authService, *m.deviceCode)
	})
}

func bootstrapCmd(ctx context.Context, runtime *app.Runtime, service AuthService) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		session, err := service.Bootstrap(ctx)
		return bootstrapMsg{session: session, err: err}
	})
}

func authTokenCmd(ctx context.Context, runtime *app.Runtime, service AuthService, token string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		session, err := service.AuthenticateWithToken(ctx, token, true)
		return authMsg{session: session, err: err}
	})
}

func startDeviceCmd(ctx context.Context, runtime *app.Runtime, service AuthService) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		code, err := service.StartDeviceFlow(ctx)
		return deviceStartMsg{code: code, err: err}
	})
}

func pollDeviceCmd(ctx context.Context, runtime *app.Runtime, service AuthService, code models.DeviceCode) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		session, err := service.CompleteDeviceFlow(ctx, code)
		return devicePollMsg{session: session, err: err}
	})
}
