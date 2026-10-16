package app

import (
	"context"
	"errors"
	"sync"
)

var ErrClosed = errors.New("application runtime is closed")

// Runtime owns admission, cancellation and draining of application operations.
type Runtime struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	stopParent func() bool
	closed     bool
	wg         sync.WaitGroup
	done       chan struct{}
}

func NewRuntime(parent context.Context) *Runtime {
	ctx, cancel := context.WithCancel(parent)
	r := &Runtime{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	r.mu.Lock()
	r.stopParent = context.AfterFunc(parent, r.Stop)
	r.mu.Unlock()
	return r
}

func (r *Runtime) Context() context.Context { return r.ctx }

// Do runs in the calling goroutine, but is registered before admission unlocks.
func (r *Runtime) Do(ctx context.Context, fn func(context.Context) error) error {
	r.mu.Lock()
	op, finish, err := r.admitLocked(ctx)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	r.wg.Add(1)
	r.mu.Unlock()
	defer r.wg.Done()
	defer finish()
	return fn(op)
}

// Spawn returns admission errors only; the owner reports its callback's result.
func (r *Runtime) Spawn(ctx context.Context, fn func(context.Context)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	op, finish, err := r.admitLocked(ctx)
	if err != nil {
		return err
	}
	r.wg.Go(func() {
		defer finish()
		fn(op)
	})
	return nil
}

func (r *Runtime) admitLocked(ctx context.Context) (context.Context, func(), error) {
	if r.closed || r.ctx.Err() != nil {
		return nil, nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	op, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.ctx, cancel)
	return op, func() { stop(); cancel() }, nil
}

func (r *Runtime) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	r.cancel()
	r.stopParent()
	// Admission is closed before the one shared join can observe the task set.
	go func() {
		r.wg.Wait()
		close(r.done)
	}()
}

// Wait observes the same eventual completion even if an earlier wait timed out.
// Call Stop first to close admission and begin draining.
func (r *Runtime) Wait(ctx context.Context) error {
	select {
	case <-r.done:
		return nil
	default:
	}
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
