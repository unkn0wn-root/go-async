package async

import (
	"context"
	"reflect"
)

// selectAwait provides Select with a fast-path that avoids reflect.MethodByName
// for native Task instances
func (t *Task[T]) selectAwait(ctx context.Context) (reflect.Value, error) {
	v, err := t.Await(ctx)
	return reflect.ValueOf(v), err
}
