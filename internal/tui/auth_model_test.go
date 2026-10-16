package tui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
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
