package async

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var globalTaskID uint64

// Task[T] represents a single async computation that eventually
// produces a value of type T or an error. Cancelable via Cancel() and
// awaitable via Await(ctx).
//
// A Task starts running immediately when constructed via Start. Results are
// immutable. A Task completes exactly once.
type Task[T any] struct {
	id     uint64
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	res    T
	err    error
}

// Start launches fn in its own goroutine with a child context derived from parent.
// The returned Task[T] is immediately running.
func Start[T any](parent context.Context, fn func(context.Context) (T, error)) *Task[T] {
	if parent == nil {
		parent = context.Background()
	}

	ctx, cancel := context.WithCancel(parent)
	t := &Task[T]{
		id:     atomic.AddUint64(&globalTaskID, 1),
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	go t.run(fn)
	return t
}

func (t *Task[T]) run(fn func(context.Context) (T, error)) {
	var v T
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = newPanicError(r)
		}
		// publish result then close(done) to ensure happens-before for waiters
		t.res, t.err = v, err
		t.cancel()
		close(t.done)
	}()

	v, err = fn(t.ctx)
}

// ID returns a monotonically increasing identifier for the task.
func (t *Task[T]) ID() uint64 { return t.id }

// Context returns the task's own context.
func (t *Task[T]) Context() context.Context { return t.ctx }

// Cancel requests cancellation of the task's context.
func (t *Task[T]) Cancel() { t.cancel() }

// Done returns a channel that is closed when the task completes (success, error, or cancellation).
func (t *Task[T]) Done() <-chan struct{} { return t.done }

// Await blocks until the task completes or the caller's context is cancelled.
// On success, it returns the value and a nil error. If the task failed, it
// returns the zero value with a non-nil error (which may wrap context.Canceled
// or context.DeadlineExceeded). If the caller's ctx is canceled first, it
// returns ctx.Err() and does not block further.
func (t *Task[T]) Await(ctx context.Context) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-t.done:
		return t.res, t.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// AwaitUncancelable waits for completion ignoring the caller's cancellation.
func (t *Task[T]) AwaitUncancelable() (T, error) {
	<-t.done
	return t.res, t.err
}

// Wait is like Await but discards the result and only returns an error.
func (t *Task[T]) Wait(ctx context.Context) error {
	_, err := t.Await(ctx)
	return err
}

// CancelAndWait cancels the task and waits for it to finish, returning its final error.
func (t *Task[T]) CancelAndWait() error {
	t.cancel()
	// Wait independently of the caller's context cancellation.
	return t.Wait(context.WithoutCancel(t.ctx))
}

// TryGet returns the result if the task is already complete.
// The third return value reports whether the value was present (true means
// t.done was closed). When ok==false, v and err are meaningless.
func (t *Task[T]) TryGet() (v T, err error, ok bool) {
	select {
	case <-t.done:
		return t.res, t.err, true
	default:
		var zero T
		return zero, nil, false
	}
}

// FromValue returns an already-completed successful task carrying v.
func FromValue[T any](v T) *Task[T] {
	ctx, cancel := context.WithCancel(context.Background())
	t := &Task[T]{
		id:     atomic.AddUint64(&globalTaskID, 1),
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		res:    v,
		err:    nil,
	}
	cancel()
	close(t.done)
	return t
}

// FromError returns an already-completed failed task carrying err.
func FromError[T any](err error) *Task[T] {
	ctx, cancel := context.WithCancel(context.Background())
	t := &Task[T]{
		id:     atomic.AddUint64(&globalTaskID, 1),
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		err:    err,
	}
	cancel()
	close(t.done)
	return t
}

// Delay returns a Task[struct{}] that completes after d or when ctx is cancelled.
func Delay(parent context.Context, d time.Duration) *Task[struct{}] {
	return Start(parent, func(ctx context.Context) (struct{}, error) {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
			return struct{}{}, nil
		case <-ctx.Done():
			return struct{}{}, ctx.Err()
		}
	})
}

// Completer allows external resolution of a Task[T] you create, similar to a
// Promise resolver pair in JavaScript.
type Completer[T any] struct {
	once sync.Once
	t    *Task[T]
}

// NewCompleter returns a (Completer, Task) pair. If the parent context is
// cancelled before Resolve/Reject is called, the Task completes with ctx.Err().
func NewCompleter[T any](parent context.Context) (*Completer[T], *Task[T]) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	t := &Task[T]{
		id:     atomic.AddUint64(&globalTaskID, 1),
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	c := &Completer[T]{t: t}

	// auto-complete with ctx.Err() if caller cancels first.
	go func() {
		<-ctx.Done()
		c.once.Do(func() {
			t.res, t.err = *new(T), ctx.Err()
			t.cancel()
			close(t.done)
		})
	}()

	return c, t
}

// Resolve completes the task successfully with v.
func (c *Completer[T]) Resolve(v T) bool {
	called := false
	c.once.Do(func() {
		called = true
		c.t.res, c.t.err = v, nil
		c.t.cancel()
		close(c.t.done)
	})
	return called
}

// Reject completes the task with err.
func (c *Completer[T]) Reject(err error) bool {
	if err == nil {
		err = errors.New("async: Reject called with nil error")
	}
	called := false
	c.once.Do(func() {
		called = true
		c.t.res, c.t.err = *new(T), err
		c.t.cancel()
		close(c.t.done)
	})
	return called
}
