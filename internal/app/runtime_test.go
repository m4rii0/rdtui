package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func drainRuntime(t *testing.T, runtime *Runtime) {
	t.Helper()
	t.Cleanup(func() {
		runtime.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := runtime.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
}

func TestRuntimeRejectsWorkAfterStop(t *testing.T) {
	runtime := NewRuntime(t.Context())
	drainRuntime(t, runtime)
	runtime.Stop()
	runtime.Stop()
	var calls atomic.Int32
	if err := runtime.Do(t.Context(), func(context.Context) error { calls.Add(1); return nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err := runtime.Spawn(t.Context(), func(context.Context) { calls.Add(1) }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("work admitted after stop")
	}
}

func TestRuntimeCancelsAndDrains(t *testing.T) {
	for _, source := range []string{"stop", "parent", "caller"} {
		t.Run(source, func(t *testing.T) {
			parent, cancelParent := context.WithCancel(t.Context())
			defer cancelParent()
			caller, cancelCaller := context.WithCancel(t.Context())
			defer cancelCaller()
			runtime := NewRuntime(parent)
			drainRuntime(t, runtime)
			entered, canceled := make(chan struct{}), make(chan struct{})
			if err := runtime.Spawn(caller, func(ctx context.Context) {
				close(entered)
				<-ctx.Done()
				close(canceled)
			}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-t.Context().Done():
				t.Fatal(t.Context().Err())
			}
			switch source {
			case "stop":
				runtime.Stop()
			case "parent":
				cancelParent()
			case "caller":
				cancelCaller()
			}
			select {
			case <-canceled:
			case <-t.Context().Done():
				t.Fatal(t.Context().Err())
			}
			runtime.Stop()
			if err := runtime.Wait(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeWaitDeadline(t *testing.T) {
	runtime := NewRuntime(t.Context())
	drainRuntime(t, runtime)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	if err := runtime.Spawn(t.Context(), func(context.Context) { <-release }); err != nil {
		t.Fatal(err)
	}
	runtime.Stop()
	expired, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	if err := runtime.Wait(expired); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	unblock()
	if err := runtime.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Do(t.Context(), func(context.Context) error { t.Error("reopened"); return nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestRuntimeSpawnRegistersBeforeReturning(t *testing.T) {
	runtime := NewRuntime(t.Context())
	drainRuntime(t, runtime)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	var completed atomic.Bool
	if err := runtime.Spawn(t.Context(), func(context.Context) { <-release; completed.Store(true) }); err != nil {
		t.Fatal(err)
	}
	runtime.Stop()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := runtime.Wait(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("unregistered worker: Wait = %v", err)
	}
	unblock()
	if err := runtime.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !completed.Load() {
		t.Fatal("Wait returned before spawned callback completed")
	}
}

func TestRuntimeDoTracksSynchronousWork(t *testing.T) {
	runtime := NewRuntime(t.Context())
	drainRuntime(t, runtime)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	want := errors.New("operation failed")
	go func() {
		done <- runtime.Do(t.Context(), func(ctx context.Context) error {
			close(entered)
			<-ctx.Done()
			<-release
			return want
		})
	}()
	<-entered
	runtime.Stop()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := runtime.Wait(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("synchronous work not tracked: %v", err)
	}
	// Release only after verifying Wait cannot finish early.
	unblock()
	if err := <-done; !errors.Is(err, want) {
		t.Fatal(err)
	}
}

func TestRuntimeRejectsCanceledCaller(t *testing.T) {
	runtime := NewRuntime(t.Context())
	drainRuntime(t, runtime)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := runtime.Do(ctx, func(context.Context) error { t.Error("called"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := runtime.Spawn(ctx, func(context.Context) { t.Error("called") }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestRuntimeOperationContextEndsWithWork(t *testing.T) {
	runtime := NewRuntime(t.Context())
	drainRuntime(t, runtime)
	var operation context.Context
	if err := runtime.Do(t.Context(), func(ctx context.Context) error {
		operation = ctx
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(operation.Err(), context.Canceled) {
		t.Fatal("completed operation retained its context")
	}
	if err := runtime.Context().Err(); err != nil {
		t.Fatalf("operation completion canceled application: %v", err)
	}
}
