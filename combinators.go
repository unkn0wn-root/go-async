package async

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

type selectable interface {
	selectAwait(context.Context) (reflect.Value, error)
}

// Map runs fn after t completes successfully, passing t's value and returning a new Task[U].
func Map[T, U any](parent context.Context, t *Task[T], fn func(context.Context, T) (U, error)) *Task[U] {
	return Start(parent, func(ctx context.Context) (U, error) {
		v, err := t.Await(ctx)
		if err != nil {
			var zero U
			return zero, err
		}
		return fn(ctx, v)
	})
}

// Then is a flatMap: it awaits t and, on success, calls fn to get another Task[U] and awaits it.
func Then[T, U any](parent context.Context, t *Task[T], fn func(context.Context, T) *Task[U]) *Task[U] {
	return Start(parent, func(ctx context.Context) (U, error) {
		v, err := t.Await(ctx)
		if err != nil {
			var zero U
			return zero, err
		}
		next := fn(ctx, v)
		return next.Await(ctx)
	})
}

// Catch maps an error from t into a recovered result by running fn only when t fails.
func Catch[T any](parent context.Context, t *Task[T], fn func(context.Context, error) (T, error)) *Task[T] {
	return Start(parent, func(ctx context.Context) (T, error) {
		v, err := t.Await(ctx)
		if err != nil {
			return fn(ctx, err)
		}
		return v, nil
	})
}

// Finally runs f after t settles (success or failure) and passes through the original result.
func Finally[T any](parent context.Context, t *Task[T], f func()) *Task[T] {
	return Start(parent, func(ctx context.Context) (T, error) {
		v, err := t.Await(ctx)
		defer func() {
			if r := recover(); r != nil {
				err = newPanicError(r)
			}
		}()

		f()
		return v, err
	})
}

// Settled captures the outcome of a Task in AllSettled results.
type Settled[T any] struct {
	Index int
	Value T
	Err   error
}

// All waits for all tasks to settle and returns their values and an aggregated error (if any).
// The returned slice matches the order of the input tasks. Any failures are joined via errors.Join.
//
// Waits for every task to finish. It does not short-circuit. If you want to
// cancel remaining work on first failure, use AllCancelOnError and ensure your tasks observe
// the passed-in context or that you are comfortable with Cancel() being called on them.
func All[T any](parent context.Context, tasks ...*Task[T]) ([]T, error) {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}

	n := len(tasks)
	results := make([]T, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i, t := range tasks {
		i, t := i, t
		go func() {
			defer wg.Done()
			v, err := t.Await(ctx)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = v
		}()
	}
	wg.Wait()
	return results, errors.Join(errs...)
}

// AllSettled waits for all tasks and returns their individual outcomes without aggregating errors.
func AllSettled[T any](parent context.Context, tasks ...*Task[T]) []Settled[T] {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}

	n := len(tasks)
	out := make([]Settled[T], n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i, t := range tasks {
		i, t := i, t
		go func() {
			defer wg.Done()
			v, err := t.Await(ctx)
			out[i] = Settled[T]{Index: i, Value: v, Err: err}
		}()
	}
	wg.Wait()
	return out
}

// AllCancelOnError waits for tasks, but if any task fails early it cancels the provided context for remaining
// waits and also calls Cancel() on all tasks to encourage prompt shutdown.
func AllCancelOnError[T any](parent context.Context, tasks ...*Task[T]) ([]T, error) {
	if parent == nil {
		parent = context.Background()
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	type result struct {
		idx int
		val T
		err error
	}

	n := len(tasks)
	results := make([]T, n)
	ch := make(chan result, n)

	var once sync.Once
	var firstErr error

	for i, t := range tasks {
		i, t := i, t
		go func() {
			v, err := t.Await(ctx)
			ch <- result{idx: i, val: v, err: err}
		}()
	}

	done := 0
	for done < n {
		select {
		case r := <-ch:
			done++
			if r.err != nil {
				once.Do(func() {
					firstErr = r.err
					cancel()
					// Encourage underlying tasks to stop if they respect their own contexts.
					for _, t := range tasks {
						t.Cancel()
					}
				})
			} else {
				results[r.idx] = r.val
			}
		case <-ctx.Done():
			// continue draining until done
		}
	}
	return results, firstErr
}

// Any returns the first successful result among tasks. If all tasks fail, it returns an aggregated error.
// It does not cancel the "losing" tasks - use AnyCancelOnSuccess if you want to signal cancellation.
func Any[T any](parent context.Context, tasks ...*Task[T]) (T, error) {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}

	var zero T
	if len(tasks) == 0 {
		return zero, fmt.Errorf("async: Any requires at least one task")
	}

	type outcome struct {
		val T
		err error
	}

	ch := make(chan outcome, len(tasks))
	for _, t := range tasks {
		t := t
		go func() {
			v, err := t.Await(ctx)
			ch <- outcome{val: v, err: err}
		}()
	}

	var errs []error
	for i := 0; i < len(tasks); i++ {
		o := <-ch
		if o.err == nil {
			return o.val, nil
		}
		errs = append(errs, o.err)
	}
	return zero, errors.Join(errs...)
}

// AnyCancelOnSuccess returns the first successful result and cancels other tasks (via Cancel()).
func AnyCancelOnSuccess[T any](parent context.Context, tasks ...*Task[T]) (T, error) {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}

	var zero T
	if len(tasks) == 0 {
		return zero, fmt.Errorf("async: AnyCancelOnSuccess requires at least one task")
	}

	type outcome struct {
		val T
		err error
	}

	ch := make(chan outcome, len(tasks))
	for _, t := range tasks {
		t := t
		go func() {
			v, err := t.Await(ctx)
			ch <- outcome{val: v, err: err}
		}()
	}

	var errs []error
	for i := 0; i < len(tasks); i++ {
		o := <-ch
		if o.err == nil {
			for _, t := range tasks {
				t.Cancel()
			}
			return o.val, nil
		}
		errs = append(errs, o.err)
	}
	return zero, errors.Join(errs...)
}

// Race waits for the first task to complete (success or failure) and returns its index, value, and error.
func Race[T any](parent context.Context, tasks ...*Task[T]) (winner int, v T, err error) {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}

	var zero T
	if len(tasks) == 0 {
		return -1, zero, fmt.Errorf("async: Race requires at least one task")
	}

	type outcome struct {
		idx int
		val T
		err error
	}

	ch := make(chan outcome, len(tasks))
	for i, t := range tasks {
		i, t := i, t
		go func() {
			v, err := t.Await(ctx)
			ch <- outcome{idx: i, val: v, err: err}
		}()
	}

	o := <-ch
	return o.idx, o.val, o.err
}

// RaceCancel is like Race but calls Cancel() on all other tasks once the winner is known.
func RaceCancel[T any](parent context.Context, tasks ...*Task[T]) (winner int, v T, err error) {
	idx, val, e := Race(parent, tasks...)
	for i, t := range tasks {
		if i != idx {
			t.Cancel()
		}
	}
	return idx, val, e
}

// Select is an advanced helper that returns when any of the provided tasks completes,
// along with its reflect.Value.
func Select(parent context.Context, tasks ...any) (int, reflect.Value, error) {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}

	if len(tasks) == 0 {
		return -1, reflect.Value{}, fmt.Errorf("async: Select requires at least one task")
	}

	type outcome struct {
		idx int
		val reflect.Value
		err error
	}

	ch := make(chan outcome, len(tasks))
	for i, t := range tasks {
		i, t := i, t
		go func() {
			if s, ok := t.(selectable); ok {
				val, err := s.selectAwait(ctx)
				ch <- outcome{i, val, err}
				return
			}

			switch tt := t.(type) {
			case *Task[any]:
				v, err := tt.Await(ctx)
				ch <- outcome{i, reflect.ValueOf(v), err}
			default:
				rv := reflect.ValueOf(t)
				m := rv.MethodByName("Await")
				if !m.IsValid() {
					ch <- outcome{i, reflect.Value{}, fmt.Errorf("async: Select received non-Task type")}
					return
				}

				out := m.Call([]reflect.Value{reflect.ValueOf(ctx)})
				// expect (T, error)
				if len(out) != 2 {
					ch <- outcome{i, reflect.Value{}, fmt.Errorf("async: Await signature mismatch")}
					return
				}

				val := out[0]
				var err error
				if !out[1].IsNil() {
					err = out[1].Interface().(error)
				}
				ch <- outcome{i, val, err}
			}
		}()
	}
	o := <-ch
	return o.idx, o.val, o.err
}
