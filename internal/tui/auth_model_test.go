package tui

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/pkg/models"
)

type cancelingService struct {
	AuthService
	entered chan struct{}
	calls   atomic.Int32
}

func (s *cancelingService) Bootstrap(ctx context.Context) (*models.AuthSession, error) {
	s.calls.Add(1)
	close(s.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestQuitCancelsOutstandingCommand(t *testing.T) {
	runtime := newTestRuntime(t)
	service := &cancelingService{entered: make(chan struct{})}
	m := NewModel(Dependencies{Auth: service, Runtime: runtime})
	queued := bootstrapCmd(t.Context(), runtime, service)
	done := make(chan tea.Msg, 1)
	go func() { done <- bootstrapCmd(t.Context(), runtime, service)() }()
	select {
	case <-service.entered:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	_, quit := m.handleShortcutAction(actionQuit)
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("quit command missing")
	}
	select {
	case msg := <-done:
		if !errors.Is(msg.(bootstrapMsg).err, context.Canceled) {
			t.Fatal(msg)
		}
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	late := queued().(operationErrorMsg)
	if !errors.Is(late.err, app.ErrClosed) {
		t.Fatal(late.err)
	}
	if service.calls.Load() != 1 {
		t.Fatal("request admitted after stop")
	}
}

type bootstrapRetryService struct {
	AuthService
	entered chan context.Context
	release chan struct{}
}

func (s *bootstrapRetryService) Bootstrap(ctx context.Context) (*models.AuthSession, error) {
	s.entered <- ctx
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	// Deliberately return a late success even after cancellation: the feature
	// generation, not transport cooperation, must protect the replacement screen.
	return &models.AuthSession{User: models.User{Username: "obsolete"}}, nil
}

func TestBootstrapRetrySupersededByTokenEntry(t *testing.T) {
	r := newTestRuntime(t)
	s := &bootstrapRetryService{entered: make(chan context.Context, 1), release: make(chan struct{})}
	m := NewModel(Dependencies{Runtime: r, Auth: s})
	updated, _ := m.Update(bootstrapMsg{err: &auth.CredentialPersistenceError{Err: os.ErrPermission}})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'r'})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("missing bootstrap retry")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	ctx := <-s.entered
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't'})
	m = updated.(Model)
	if ctx.Err() == nil {
		t.Fatal("replacement authentication did not cancel bootstrap retry")
	}
	close(s.release)
	updated, next := m.Update(<-done)
	m = updated.(Model)
	if m.mode != modeTokenInput || m.session != nil || m.retryBootstrap || next != nil {
		t.Fatal("obsolete bootstrap replaced token entry")
	}
}
