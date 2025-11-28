package template

import "context"

type executionContextKey struct{}

// WithExecutionContext stores v into ctx so template execution can retrieve it.
func WithExecutionContext(ctx context.Context, v any) context.Context {
	return context.WithValue(ctx, executionContextKey{}, v)
}

func getExecutionContextValue(ctx context.Context) any {
	if ctx == nil {
		return nil
	}
	return ctx.Value(executionContextKey{})
}
