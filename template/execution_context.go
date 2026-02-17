package template

import "context"

type executionContextKey struct{}

// WithExecutionContext stores v into ctx so template execution can retrieve it.
func WithExecutionContext(ctx context.Context, v any) context.Context {
	return context.WithValue(ctx, executionContextKey{}, v)
}

// ExecutionContext retrieves the value stored by WithExecutionContext.
// Returns nil if ctx is nil or no value was stored.
func ExecutionContext(ctx context.Context) any {
	if ctx == nil {
		return nil
	}
	return ctx.Value(executionContextKey{})
}
