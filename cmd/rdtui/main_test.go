package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/app"
)

type failingWriter struct {
	err error
}

func TestRuntimeCleanupUsesIndependentBudget(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	runtime := app.NewRuntime(parent)
	cancel()
	want := errors.New("backend close failed")
	err := cleanupRuntime(runtime, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			t.Fatalf("cleanup inherited canceled root: %v", err)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Fatal("cleanup is not bounded")
		}
		if err := runtime.Do(t.Context(), func(context.Context) error { t.Error("admitted"); return nil }); !errors.Is(err, app.ErrClosed) {
			t.Fatal(err)
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
}

func TestRuntimeCleanupPreservesCloseAndDrainErrors(t *testing.T) {
	runtime := app.NewRuntime(t.Context())
	release := make(chan struct{})
	defer close(release)
	t.Cleanup(func() {
		runtime.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := runtime.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
	if err := runtime.Spawn(t.Context(), func(context.Context) { <-release }); err != nil {
		t.Fatal(err)
	}
	want := errors.New("backend close failed")
	err := cleanupRuntime(runtime, func(ctx context.Context) error {
		<-ctx.Done()
		return want
	})
	if !errors.Is(err, want) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost cleanup error: %v", err)
	}
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestPrintVersionPropagatesWriteError(t *testing.T) {
	want := errors.New("output unavailable")
	if err := printVersion(failingWriter{err: want}); !errors.Is(err, want) {
		t.Fatalf("printVersion error = %v, want %v", err, want)
	}
}

func TestRunReturnsErrorWhenVersionOutputFails(t *testing.T) {
	var stderr bytes.Buffer
	want := errors.New("output unavailable")
	if code := run([]string{"--version"}, failingWriter{err: want}, &stderr); code != 1 {
		t.Fatalf("run exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), want.Error()) {
		t.Fatalf("stderr = %q, want output error", stderr.String())
	}
}

func TestRunPreservesUsageExitCodeWhenStderrFails(t *testing.T) {
	var stdout bytes.Buffer
	stderr := failingWriter{err: errors.New("stderr unavailable")}
	if code := run([]string{"update", "--unknown"}, &stdout, stderr); code != 2 {
		t.Fatalf("run exit code = %d, want 2", code)
	}
}
