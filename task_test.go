package async

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStartAwaitSuccess(t *testing.T) {
	ctx := context.Background()
	tk := Start(ctx, func(ctx context.Context) (int, error) {
		return 123, nil
	})
	v, err := tk.Await(ctx)
	if err != nil || v != 123 {
		t.Fatalf("got %v, %v", v, err)
	}
}

func TestCancel(t *testing.T) {
	ctx := context.Background()
	tk := Start(ctx, func(ctx context.Context) (int, error) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return 1, nil
		}
	})
	tk.Cancel()
	_, err := tk.Await(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled; got %v", err)
	}
}

func TestMapThenCatchFinally(t *testing.T) {
	ctx := context.Background()
	base := Start(ctx, func(ctx context.Context) (int, error) { return 2, nil })
	mapped := Map(ctx, base, func(ctx context.Context, v int) (int, error) { return v * 2, nil })
	then := Then(ctx, mapped, func(ctx context.Context, v int) *Task[int] { return FromValue(v + 1) })
	final := Finally(ctx, then, func() {})
	v, err := final.Await(ctx)
	if err != nil || v != 5 {
		t.Fatalf("got %v, %v", v, err)
	}
}

func TestAllAggregatesErrors(t *testing.T) {
	ctx := context.Background()
	ok := FromValue(1)
	fail := FromError[int](errors.New("x"))
	_, err := All(ctx, ok, fail)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestAnyAllFail(t *testing.T) {
	ctx := context.Background()
	a := FromError[int](errors.New("a"))
	b := FromError[int](errors.New("b"))
	_, err := Any[int](ctx, a, b)
	if err == nil {
		t.Fatalf("expected aggregated error")
	}
}

func mustContextDone(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("context done not closed")
	}
}

func TestTaskContextDoneClosesOnCompletion(t *testing.T) {
	ctx := context.Background()
	task := Start(ctx, func(ctx context.Context) (int, error) {
		return 42, nil
	})
	if _, err := task.Await(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mustContextDone(t, task.Context())
}

func TestCompleterResolveClosesContext(t *testing.T) {
	ctx := context.Background()
	c, task := NewCompleter[int](ctx)
	if ok := c.Resolve(7); !ok {
		t.Fatalf("expected resolver to succeed")
	}
	mustContextDone(t, task.Context())
	if v, err := task.Await(ctx); err != nil || v != 7 {
		t.Fatalf("got %v, %v", v, err)
	}
}

func TestFromValueContextClosed(t *testing.T) {
	task := FromValue(1)
	mustContextDone(t, task.Context())
	if v, err := task.Await(context.Background()); err != nil || v != 1 {
		t.Fatalf("got %v, %v", v, err)
	}
}

func TestFromErrorContextClosed(t *testing.T) {
	task := FromError[int](errors.New("boom"))
	mustContextDone(t, task.Context())
	if _, err := task.Await(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}
