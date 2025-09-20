# typed Tasks with async/await-like for Go

> **TL;DR**: This package provides a small `Task[T]` type
> with `Await`, cancellation, panic-to-error conversion, and Promise-style
> combinators like `Map`, `Then`, `Catch`, `Finally`, `All`, `Any`, and `Race`.
> Context-aware and avoids goroutine leaks.

**Why**: I like to create stuff without any particular reason.

## Install

```bash
go get github.com/unkn0wn-root/go-async/async
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

### Combinators

- `Map`, `Then`, `Catch`, `Finally`
- `All` / `AllCancelOnError`
- `AllSettled`
- `Any` (first success)
- `Race` (first completion)
- `Delay`
- `NewCompleter` (externally resolve/reject a Task)

See the examples in `async/examples_test.go`.

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

- This is not a replacement for channels at all. Prefer channels when streaming many
  results. Use `Task` for one-shot, pretend this is JS world, composition-friendly async values.
- For best cancellation behavior in groups, create child tasks with the same
  parent `ctx` you pass into combinators.
