package reporter

import (
	"context"

	"github.com/cenkalti/backoff/v4"
)

// RunWithRetry runs f as a subtest of r called name with retry.
func RunWithRetry(ctx context.Context, r Reporter, name string, f func(Reporter), policy RetryPolicy) bool {
	return r.runWithRetry(name, f, policy)
}

// RetryPolicy is an interface for the retry backoff policies.
type RetryPolicy interface {
	Build(context.Context) (context.Context, func(), backoff.BackOff, error)
}
