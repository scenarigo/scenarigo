package schema

import (
	"context"
	"errors"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// RetryPolicy represents a retry policy.
type RetryPolicy struct {
	Constant    *RetryPolicyConstant    `yaml:"constant,omitempty"`
	Exponential *RetryPolicyExponential `yaml:"exponential,omitempty"`
}

// Build returns p as backoff.BackOff.
// If p is nil, Build returns the policy which never retry.
func (p *RetryPolicy) Build(ctx context.Context) (context.Context, func(), backoff.BackOff, error) {
	if p != nil {
		if p.Constant != nil && p.Exponential != nil {
			return nil, nil, nil, errors.New("ambiguous retry policy")
		}
		if p.Constant != nil {
			return p.Constant.Build(ctx)
		}
		if p.Exponential != nil {
			return p.Exponential.Build(ctx)
		}
	}
	return ctx, func() {}, &backoff.StopBackOff{}, nil
}

func maxElapsedTimeContextFunc(ctx context.Context, t *Duration, b backoff.BackOff) (context.Context, func(), backoff.BackOff) {
	if t == nil || *t == 0 {
		return ctx, func() {}, b
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(*t))
	return ctx, cancel, backoff.WithContext(b, ctx)
}

// RetryPolicyConstant represents a constant retry policy.
type RetryPolicyConstant struct {
	Interval       *Duration `yaml:"interval,omitempty"`       // default value is 1s
	MaxRetries     *int      `yaml:"maxRetries,omitempty"`     // default value is 5, 0 means forever
	MaxElapsedTime *Duration `yaml:"maxElapsedTime,omitempty"` // default value is 0, 0 means forever
}

// Build returns p as backoff.Policy.
func (p *RetryPolicyConstant) Build(ctx context.Context) (context.Context, func(), backoff.BackOff, error) {
	interval := time.Second
	if p.Interval != nil {
		interval = time.Duration(*p.Interval)
	}
	var b backoff.BackOff = backoff.NewConstantBackOff(interval)

	maxRetries := 5
	if p.MaxRetries != nil && *p.MaxRetries >= 0 {
		maxRetries = *p.MaxRetries
	}
	if maxRetries > 0 {
		b = backoff.WithMaxRetries(b, uint64(maxRetries))
	}

	ctx, cancel, b := maxElapsedTimeContextFunc(ctx, p.MaxElapsedTime, b)
	return ctx, cancel, b, nil
}

// RetryPolicyExponential represents a exponential retry policy.
type RetryPolicyExponential struct {
	InitialInterval *Duration `yaml:"initialInterval,omitempty"` // default value is 500ms
	Factor          *float64  `yaml:"factor,omitempty"`          // default value is 1.5
	JitterFactor    *float64  `yaml:"jitterFactor,omitempty"`    // default value is 0.5
	MaxInterval     *Duration `yaml:"maxInterval,omitempty"`     // default value is 60s
	MaxRetries      *int      `yaml:"maxRetries,omitempty"`      // default value is 5, 0 means forever
	MaxElapsedTime  *Duration `yaml:"maxElapsedTime,omitempty"`  // default value is 0, 0 means forever
}

// Build returns p as backoff.Policy.
func (p *RetryPolicyExponential) Build(ctx context.Context) (context.Context, func(), backoff.BackOff, error) {
	eb := backoff.NewExponentialBackOff()
	if p.InitialInterval != nil {
		eb.InitialInterval = time.Duration(*p.InitialInterval)
	}
	if p.Factor != nil {
		eb.Multiplier = *p.Factor
	}
	if p.JitterFactor != nil {
		eb.RandomizationFactor = *p.JitterFactor
	}
	if p.MaxInterval != nil {
		eb.MaxInterval = time.Duration(*p.MaxInterval)
	}
	eb.MaxElapsedTime = 0
	if p.MaxElapsedTime != nil {
		eb.MaxElapsedTime = time.Duration(*p.MaxElapsedTime)
	}

	var b backoff.BackOff = eb
	maxRetries := 5
	if p.MaxRetries != nil && *p.MaxRetries >= 0 {
		maxRetries = *p.MaxRetries
	}
	if maxRetries > 0 {
		b = backoff.WithMaxRetries(b, uint64(maxRetries))
	}

	ctx, cancel, b := maxElapsedTimeContextFunc(ctx, p.MaxElapsedTime, b)
	return ctx, cancel, b, nil
}
