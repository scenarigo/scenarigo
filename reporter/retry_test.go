package reporter

import (
	"context"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
)

func TestRunWithRetry(t *testing.T) {
	var ok bool
	var i int
	r := FromT(t)
	r.Run("wrapper for waiting", func(r Reporter) {
		policy := &constantRetryPolicy{
			interval:   100 * time.Millisecond,
			maxRetries: 1,
		}
		ok = RunWithRetry(context.Background(), r, "run with retry", func(r Reporter) {
			r.Parallel()
			i++
			if i < 2 {
				r.Fatal("fail")
			}
			r.Log("pass")
		}, policy)
	})
	if got, expect := i, 2; got != expect {
		r.Fatalf("expect %d but got %d", expect, got)
	}
	if got, expect := ok, true; got != expect {
		r.Fatalf("expect %t but got %t", expect, got)
	}
}

func TestRunWithRetry_Parallel(t *testing.T) {
	ctx := context.Background()
	retryPolicy := &constantRetryPolicy{
		interval:   time.Microsecond,
		maxRetries: 2,
	}
	t.Run("retry/nil/nil", func(t *testing.T) {
		r := FromT(t)
		var i int
		RunWithRetry(ctx, r, "a", func(r Reporter) {
			RunWithRetry(ctx, r, "b", func(r Reporter) {
				RunWithRetry(ctx, r, "c", func(r Reporter) {
					r.Parallel()
					i++
					if i < 3 {
						r.Error("error")
					}
				}, nil)
			}, nil)
		}, retryPolicy)
	})
	t.Run("nil/retry/nil", func(t *testing.T) {
		r := FromT(t)
		var i int
		RunWithRetry(ctx, r, "a", func(r Reporter) {
			RunWithRetry(ctx, r, "b", func(r Reporter) {
				RunWithRetry(ctx, r, "c", func(r Reporter) {
					r.Parallel()
					i++
					if i < 3 {
						r.Error("error")
					}
				}, nil)
			}, retryPolicy)
		}, nil)
	})
	t.Run("nil/nil/retry", func(t *testing.T) {
		r := FromT(t)
		var i int
		RunWithRetry(ctx, r, "a", func(r Reporter) {
			RunWithRetry(ctx, r, "b", func(r Reporter) {
				RunWithRetry(ctx, r, "c", func(r Reporter) {
					r.Parallel()
					i++
					if i < 3 {
						r.Error("error")
					}
				}, retryPolicy)
			}, nil)
		}, nil)
	})
	t.Run("retry/retry/retry", func(t *testing.T) {
		r := FromT(t)
		var i int
		RunWithRetry(ctx, r, "a", func(r Reporter) {
			RunWithRetry(ctx, r, "b", func(r Reporter) {
				RunWithRetry(ctx, r, "c", func(r Reporter) {
					r.Parallel()
					i++
					if i < 27 {
						r.Error("error")
					}
				}, retryPolicy)
			}, retryPolicy)
		}, retryPolicy)
	})
}

type constantRetryPolicy struct {
	interval       time.Duration
	maxRetries     uint64
	maxElapsedTime time.Duration
}

func (p *constantRetryPolicy) Build(ctx context.Context) (context.Context, func(), backoff.BackOff, error) {
	b := backoff.WithMaxRetries(backoff.NewConstantBackOff(p.interval), p.maxRetries)
	cancel := func() {}
	if p.maxElapsedTime > 0 {
		ctx, cancel = context.WithTimeout(ctx, p.maxElapsedTime)
		b = backoff.WithContext(b, ctx)
	}
	return ctx, cancel, b, nil
}
