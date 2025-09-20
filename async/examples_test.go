package async_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/unkn0wn-root/go-async/async"
)

type User struct {
	ID   int
	Name string
}

func loadUser(ctx context.Context, id int) (User, error) {
	select {
	case <-time.After(50 * time.Millisecond):
		return User{ID: id, Name: "Ada"}, nil
	case <-ctx.Done():
		return User{}, ctx.Err()
	}
}

func Example_basic() {
	ctx := context.Background()
	t := async.Start(ctx, func(ctx context.Context) (User, error) {
		return loadUser(ctx, 42)
	})
	u, err := t.Await(ctx)
	fmt.Println(u.Name, err == nil)
	// Output:
	// Ada true
}

func Example_chainMapThen() {
	ctx := context.Background()
	load := async.Start(ctx, func(ctx context.Context) (User, error) {
		return loadUser(ctx, 1)
	})

	upper := async.Map(ctx, load, func(ctx context.Context, u User) (string, error) {
		return strings.ToUpper(u.Name), nil
	})
	length := async.Map(ctx, upper, func(ctx context.Context, s string) (int, error) {
		return len(s), nil
	})
	n, _ := length.Await(ctx)
	fmt.Println(n > 0)
	// Output:
	// true
}

func Example_groupAll() {
	ctx := context.Background()
	a := async.Delay(ctx, 10*time.Millisecond)
	b := async.Delay(ctx, 20*time.Millisecond)
	c := async.Delay(ctx, 5*time.Millisecond)
	_, err := async.All(ctx, a, b, c)
	fmt.Println(err == nil)
	// Output:
	// true
}

func Example_any() {
	ctx := context.Background()
	failFast := async.Start(ctx, func(ctx context.Context) (int, error) {
		return 0, errors.New("boom")
	})
	succeedSlow := async.Start(ctx, func(ctx context.Context) (int, error) {
		time.Sleep(20 * time.Millisecond)
		return 7, nil
	})
	v, err := async.Any[int](ctx, failFast, succeedSlow)
	fmt.Println(v, err == nil)
	// Output:
	// 7 true
}

func Example_completer() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, t := async.NewCompleter[int](ctx)

	_, err := t.Await(context.Background())
	fmt.Println(errors.Is(err, context.DeadlineExceeded))
	// Output:
	// true
}
