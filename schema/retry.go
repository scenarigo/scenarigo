package schema

import (
	"context"
	"time"

	"github.com/lestrrat-go/backoff"
	"golang.org/x/xerrors"
)

// RetryPolicy represents a retry policy.
type RetryPolicy struct {
	Constant    *RetryPolicyConstant    `yaml:"constant"`
	Exponential *RetryPolicyExponential `yaml:"exponential"`
}

// Build returns p as backoff.Policy.
// If p is nil, Build returns the policy which never retry.
func (p *RetryPolicy) Build() (backoff.Policy, error) {
	if p != nil {
		if p.Constant != nil && p.Exponential != nil {
			return nil, xerrors.New("ambiguous retry policy")
		}
		if p.Constant != nil {
			return p.Constant.Build()
		}
		if p.Exponential != nil {
			return p.Exponential.Build()
		}
	}
	return &retryPolicyNever{}, nil
}

// RetryPolicyConstant represents a constant retry policy.
type RetryPolicyConstant struct {
	Interval       string  `yaml:"interval"`
	MaxElapsedTime *string `yaml:"maxElapsedTime"`
	MaxRetries     *int    `yaml:"maxRetries"`
}

// Build returns p as backoff.Policy.
func (p *RetryPolicyConstant) Build() (backoff.Policy, error) {
	interval, err := time.ParseDuration(p.Interval)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse interval: %w", err)
	}
	opts := []backoff.Option{}
	if p.MaxElapsedTime != nil {
		t, err := time.ParseDuration(*p.MaxElapsedTime)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse max elapsed time: %w", err)
		}
		opts = append(opts, backoff.WithMaxElapsedTime(t))
	}
	if p.MaxRetries != nil {
		opts = append(opts, backoff.WithMaxRetries(*p.MaxRetries))
	}
	return backoff.NewConstant(interval, opts...), nil
}

// RetryPolicyExponential represents a exponential retry policy.
type RetryPolicyExponential struct {
	InitialInterval *string  `yaml:"initialInterval"`
	Factor          *float64 `yaml:"factor"`
	JitterFactor    *float64 `yaml:"jitterFactor"`
	MaxInterval     *string  `yaml:"maxInterval"`
	MaxElapsedTime  *string  `yaml:"maxElapsedTime"`
	MaxRetries      *int     `yaml:"maxRetries"`
}

// Build returns p as backoff.Policy.
func (p *RetryPolicyExponential) Build() (backoff.Policy, error) {
	opts := []backoff.Option{}
	if p.InitialInterval != nil {
		t, err := time.ParseDuration(*p.InitialInterval)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse max elapsed time: %w", err)
		}
		opts = append(opts, backoff.WithInterval(t))
	}
	if p.Factor != nil {
		opts = append(opts, backoff.WithFactor(*p.Factor))
	}
	if p.JitterFactor != nil {
		opts = append(opts, backoff.WithJitterFactor(*p.JitterFactor))
	}
	if p.MaxInterval != nil {
		t, err := time.ParseDuration(*p.MaxInterval)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse max interval: %w", err)
		}
		opts = append(opts, backoff.WithMaxInterval(t))
	}
	if p.MaxElapsedTime != nil {
		t, err := time.ParseDuration(*p.MaxElapsedTime)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse max elapsed time: %w", err)
		}
		opts = append(opts, backoff.WithMaxElapsedTime(t))
	}
	if p.MaxRetries != nil {
		opts = append(opts, backoff.WithMaxRetries(*p.MaxRetries))
	}
	return backoff.NewExponential(opts...), nil
}

type retryPolicyNever struct{}

// Start implements backoff.Policy interface.
func (*retryPolicyNever) Start(ctx context.Context) (backoff.Backoff, backoff.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	b := &retryBackoffNever{
		ctx:    ctx,
		cancel: cancel,
		next:   make(chan struct{}),
	}
	go func() {
		b.next <- struct{}{}
		cancel()
	}()
	return b, backoff.CancelFunc(func() {
		cancel()
	})
}

type retryBackoffNever struct {
	ctx    context.Context
	cancel func()
	next   chan struct{}
}

// Done implements backoff.Backoff interface.
func (b *retryBackoffNever) Done() <-chan struct{} {
	return b.ctx.Done()
}

// Next implements backoff.Backoff interface.
func (b *retryBackoffNever) Next() <-chan struct{} {
	return b.next
}
