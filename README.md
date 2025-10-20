# typed Tasks with async/await-like for Go

> **TL;DR**: This package provides a small `Task[T]` type
> with `Await`, cancellation, panic-to-error conversion, and Promise-style
> combinators like `Map`, `Then`, `Catch`, `Finally`, `All`, `Any`, and `Race`.
> Context-aware and avoids goroutine leaks.

**Why**: I like to create stuff without any particular reason.

## Install

```bash
go get github.com/unkn0wn-root/go-async
```

## Quick start

```go
ctx := context.Background()

userTask := async.Start(ctx, func(ctx context.Context) (User, error) {
    // expensive work...
    return loadUser(ctx, 42)
})

// Await with cancellation from caller:
user, err := userTask.Await(ctx)
if err != nil { /* handle */ }

// Chain transformations:
nameTask := async.Map(ctx, userTask, func(ctx context.Context, u User) (string, error) {
    return strings.ToUpper(u.Name), nil
})
name, _ := nameTask.Await(ctx)
```

## Channels vs Tasks

Channels are perfect for streaming many values or building long-lived pipelines. `Task[T]` is good when you're brave enough to adapt `JS/C#` like async patterns and/or want single-result computations with cancellation, panic-to-error conversion and so on. (I'm not responsible for any critics you'll get (and you will) from Gophers)

### Single async operation

**Without go-async**

```go
ctx, cancel := context.WithTimeout(context.Background(), time.Second)
defer cancel()

type userResult struct {
    user User
    err  error
}

resultCh := make(chan userResult, 1)

go func() {
    user, err := loadUser(ctx, 42)
    resultCh <- userResult{user: user, err: err}
}()

var user User
var err error

select {
case out := <-resultCh:
    user, err = out.user, out.err
case <-ctx.Done():
    err = ctx.Err()
}
```

**With go-async**

```go
ctx := context.Background()

userTask := async.Start(ctx, func(ctx context.Context) (User, error) {
    return loadUser(ctx, 42)
})

user, err := userTask.Await(ctx)
```

### Transform a result

**Without go-async**

```go
resCh := make(chan struct {
    name string
    err  error
}, 1)

go func() {
    user, err := loadUser(ctx, 42)
    if err != nil {
        resCh <- struct {
            name string
            err  error
        }{"", err}
        return
    }
    resCh <- struct {
        name string
        err  error
    }{strings.ToUpper(user.Name), nil}
}()

out := <-resCh
name, err := out.name, out.err
```

**With go-async**

```go
userTask := async.Start(ctx, loadUser)

nameTask := async.Map(ctx, userTask, func(ctx context.Context, u User) (string, error) {
    return strings.ToUpper(u.Name), nil
})

name, err := nameTask.Await(ctx)
```

### fan-out with early cancellation

**Without go-async**

```go
ctx, cancel := context.WithCancel(parent)
defer cancel()

var wg sync.WaitGroup
errs := make(chan error, len(endpoints))
values := make([]Data, len(endpoints))

for i, ep := range endpoints {
    i, ep := i, ep
    wg.Add(1)
    go func() {
        defer wg.Done()
        val, err := fetch(ctx, ep)
        if err != nil {
            errs <- err
            cancel()
            return
        }
        values[i] = val
    }()
}

go func() {
    wg.Wait()
    close(errs)
}()

for err := range errs {
    if err != nil {
        return nil, err
    }
}

return values, nil
```

**With go-async**

```go
tasks := make([]*async.Task[Data], len(endpoints))

for i, ep := range endpoints {
    endpoint := ep
    tasks[i] = async.Start(ctx, func(ctx context.Context) (Data, error) {
        return fetch(ctx, endpoint)
    })
}

values, err := async.AllCancelOnError(ctx, tasks...)
```

### What you gain

- Less boilerplate for single-result async flows; no custom structs or select loops.
- Built-in composition helpers for mapping, chaining, and coordinating multiple results.
- Context-aware cancellation and panic-to-error conversion baked into every task.
- "Beloved" async/await pattern from JS/C# (I don't judge)

### Combinators

- `Map`, `Then`, `Catch`, `Finally`
- `All` / `AllCancelOnError`
- `AllSettled`
- `Any` (first success)
- `Race` (first completion)
- `Delay`
- `NewCompleter` (externally resolve/reject a Task)

See the examples in `examples_test.go`.

## Notes

- **Context-propagation**: every `Task` runs with its own derived context.
- **Cancellation**: `Task.Cancel()` cancels the task’s context, `Await(ctx)` also
  obeys the caller’s context (dual cancellation).
- **No leaks**: all `Await`-based combinators use the caller’s context to stop
  waiting; functions suffixed with `Cancel` will also explicitly call `Cancel()`
  on remaining tasks to encourage prompt shutdown.
- **Panics become errors** (`*async.PanicError`) with stack trace.
- **Race-safety**: all result writes happen-before `Done` is closed.
- **Zero dependencies**: only standard library.

## Caveats

- This is not a replacement for channels at all and I don't try to replace anything. Use what you're most comfortable with.
- For best cancellation behavior in groups, create child tasks with the same
  parent `ctx` you pass into combinators.
