package async

import (
	"fmt"
	"runtime/debug"
)

type PanicError struct {
	Value any
	Stack []byte
}

func (p *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", p.Value)
}

func newPanicError(v any) *PanicError {
	return &PanicError{Value: v, Stack: debug.Stack()}
}
