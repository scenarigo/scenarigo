package template

import (
	"context"

	sccontext "github.com/scenarigo/scenarigo/context"
)

type executionContextKey struct{}

// WithExecutionContext stores c into ctx so template execution can retrieve it.
func WithExecutionContext(ctx context.Context, c *sccontext.Context) context.Context {
	return context.WithValue(ctx, executionContextKey{}, c)
}

// ExecutionContext retrieves the *sccontext.Context stored by WithExecutionContext.
// Returns nil if ctx is nil or no value was stored.
func ExecutionContext(ctx context.Context) *sccontext.Context {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(executionContextKey{}).(*sccontext.Context)
	return c
}
