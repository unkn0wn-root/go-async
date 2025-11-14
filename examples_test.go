package async_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/unkn0wn-root/go-async"
)

func ExampleStart() {
	ctx := context.Background()
	t := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(20 * time.Millisecond)
		return 42, nil
	})

	v, err := t.Await(ctx)
	fmt.Println(v, err == nil)
	// Output:
	// 42 true
}

func ExampleTask_Await_callerTimeout() {
	ctx := context.Background()
	t := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(50 * time.Millisecond)
		return 1, nil
	})

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Millisecond)
	defer cancel()

	_, err := t.Await(ctx2)
	fmt.Println(errors.Is(err, context.DeadlineExceeded))
	// Output:
	// true
}

func ExampleTask_AwaitUncancelable() {
	ctx := context.Background()
	t := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(15 * time.Millisecond)
		return 7, nil
	})

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Millisecond)
	defer cancel()

	_, err := t.Await(ctx2)
	fmt.Println("await err is deadline:", errors.Is(err, context.DeadlineExceeded))
	v, err := t.AwaitUncancelable()
	fmt.Println("uncancelable", v, err == nil)
	// Output:
	// await err is deadline: true
	// uncancelable 7 true
}

func ExampleTask_Wait() {
	ctx := context.Background()
	t := async.FromValue(123)
	err := t.Wait(ctx)
	fmt.Println(err == nil)
	// Output:
	// true
}

func ExampleTask_Cancel() {
	ctx := context.Background()
	t := async.Start(ctx, func(ctx context.Context) (int, error) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return 99, nil
		}
	})
	t.Cancel()
	_, err := t.Await(context.Background())
	fmt.Println(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
	// Output:
	// true
}

func ExampleTask_CancelAndWait() {
	ctx := context.Background()
	t := async.Start(ctx, func(ctx context.Context) (int, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	})

	err := t.CancelAndWait()
	fmt.Println(errors.Is(err, context.Canceled))
	// Output:
	// true
}

func ExampleTask_TryGet() {
	ctx := context.Background()
	t := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(15 * time.Millisecond)
		return 11, nil
	})

	_, _, ok := t.TryGet()
	fmt.Println("ready?", ok)
	_, _ = t.AwaitUncancelable()
	_, _, ok = t.TryGet()
	fmt.Println("ready?", ok)
	// Output:
	// ready? false
	// ready? true
}

func ExampleTask_Done() {
	t := async.FromValue("x")
	<-t.Done()
	v, err := t.Await(context.Background())
	fmt.Println(v, err == nil)
	// Output:
	// x true
}

func ExampleTask_Context() {
	t := async.FromValue("id")
	fmt.Println(t.Context() != nil)
	// Output:
	// true
}

func ExampleFromValue_andFromError() {
	ctx := context.Background()
	ok := async.FromValue("ready")
	s, err := ok.Await(ctx)
	fmt.Println(s, err == nil)

	bad := async.FromError[int](errors.New("boom"))
	_, err = bad.Await(ctx)
	fmt.Println(err != nil)
	// Output:
	// ready true
	// true
}

func ExampleDelay() {
	ctx := context.Background()
	d := async.Delay(ctx, 5*time.Millisecond)
	_, err := d.Await(ctx)
	fmt.Println(err == nil)
	// Output:
	// true
}

func ExampleMap() {
	ctx := context.Background()
	base := async.FromValue(21)
	doubled := async.Map(ctx, base, func(ctx context.Context, v int) (int, error) {
		return v * 2, nil
	})

	v, _ := doubled.Await(ctx)
	fmt.Println(v)
	// Output:
	// 42
}

func ExampleThen() {
	ctx := context.Background()
	base := async.FromValue(10)
	next := async.Then(ctx, base, func(ctx context.Context, v int) *async.Task[int] {
		return async.FromValue(v + 5)
	})

	v, _ := next.Await(ctx)
	fmt.Println(v)
	// Output:
	// 15
}

func ExampleCatch() {
	ctx := context.Background()
	bad := async.FromError[int](errors.New("nope"))
	fixed := async.Catch(ctx, bad, func(ctx context.Context, err error) (int, error) {
		return 7, nil
	})

	v, err := fixed.Await(ctx)
	fmt.Println(v, err == nil)
	// Output:
	// 7 true
}

func ExampleFinally() {
	ctx := context.Background()
	flag := ""
	t := async.Finally(ctx, async.FromValue(1), func() { flag = "ran" })
	v, err := t.Await(ctx)
	fmt.Println(v, err == nil, flag)
	// Output:
	// 1 true ran
}

func ExampleAll() {
	ctx := context.Background()
	a := async.FromValue(1)
	b := async.FromValue(2)
	c := async.FromError[int](errors.New("c failed"))
	vals, err := async.All(ctx, a, b, c)
	fmt.Println(vals[0], vals[1], vals[2], err != nil)
	// Output:
	// 1 2 0 true
}

func ExampleAllSettled() {
	ctx := context.Background()
	a := async.FromValue(1)
	b := async.FromError[int](errors.New("b failed"))
	out := async.AllSettled(ctx, a, b)
	fmt.Println(out[0].Err == nil, out[1].Err != nil)
	// Output:
	// true true
}

func ExampleAllCancelOnError() {
	ctx := context.Background()
	slow := async.Start(ctx, func(ctx context.Context) (int, error) {
		select {
		case <-time.After(300 * time.Millisecond):
			return 3, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	})

	failFast := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(20 * time.Millisecond)
		return 0, errors.New("boom")
	})

	_, err := async.AllCancelOnError(ctx, slow, failFast)
	fmt.Println(err != nil)
	// Output:
	// true
}

func ExampleAny() {
	ctx := context.Background()
	bad := async.FromError[int](errors.New("a"))
	good := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(30 * time.Millisecond)
		return 99, nil
	})

	v, err := async.Any[int](ctx, bad, good)
	fmt.Println(v, err == nil)
	// Output:
	// 99 true
}

func ExampleAnyCancelOnSuccess() {
	ctx := context.Background()
	bad := async.FromError[int](errors.New("a"))
	good := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(10 * time.Millisecond)
		return 77, nil
	})

	v, err := async.AnyCancelOnSuccess[int](ctx, bad, good)
	fmt.Println(v, err == nil)
	// Output:
	// 77 true
}

func ExampleRace() {
	ctx := context.Background()
	fastFail := async.FromError[int](errors.New("fail"))
	slowOk := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(40 * time.Millisecond)
		return 1, nil
	})

	idx, v, err := async.Race(ctx, slowOk, fastFail)
	fmt.Println(idx, v, err != nil)
	// Output:
	// 1 0 true
}

func ExampleRaceCancel() {
	ctx := context.Background()
	slow1 := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(50 * time.Millisecond)
		return 1, nil
	})

	fast := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(10 * time.Millisecond)
		return 2, nil
	})

	idx, v, err := async.RaceCancel(ctx, slow1, fast)
	fmt.Println(idx, v, err == nil)
	// Output:
	// 1 2 true
}

func ExampleNewCompleter() {
	ctx := context.Background()
	comp1, t1 := async.NewCompleter[int](ctx)
	comp2, t2 := async.NewCompleter[int](ctx)

	go func() { comp1.Resolve(5) }()
	go func() { comp2.Reject(errors.New("x")) }()

	v1, err1 := t1.Await(ctx)
	_, err2 := t2.Await(ctx)
	fmt.Println(v1, err1 == nil, err2 != nil)
	// Output:
	// 5 true true
}

func ExamplePanicError() {
	ctx := context.Background()
	t := async.Start[int](ctx, func(ctx context.Context) (int, error) {
		panic("unexpected")
	})

	_, err := t.Await(ctx)
	var p *async.PanicError
	fmt.Println(errors.As(err, &p), p != nil && p.Value == "unexpected")
	// Output:
	// true true
}

func ExampleSelect() {
	ctx := context.Background()
	t1 := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(25 * time.Millisecond)
		return 123, nil
	})

	t2 := async.Start(ctx, func(ctx context.Context) (string, error) {
		time.Sleep(5 * time.Millisecond)
		return "hello", nil
	})

	idx, val, err := async.Select(ctx, t1, t2)
	fmt.Println(idx, val.Interface() == "hello", err == nil)
	// Output:
	// 1 true true
}
