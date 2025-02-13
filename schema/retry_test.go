package schema

import (
	"context"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/scenarigo/scenarigo/errors"
)

func TestRetryPolicyConstant(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		p := &RetryPolicy{
			Constant: &RetryPolicyConstant{},
		}
		_, cancel, b, err := p.Build(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer cancel()

		var i int
		_ = backoff.Retry(func() error {
			i++
			return errors.New("retry")
		}, b)
		if got, expect := i, 6; got != expect {
			t.Errorf("expect %d but got %d", expect, got)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		maxElapsedTime := time.Millisecond
		p := &RetryPolicy{
			Constant: &RetryPolicyConstant{
				MaxElapsedTime: (*Duration)(&maxElapsedTime),
			},
		}
		_, cancel, b, err := p.Build(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer cancel()

		var i int
		_ = backoff.Retry(func() error {
			i++
			return errors.New("retry")
		}, b)
		if got, expect := i, 1; got != expect {
			t.Errorf("expect %d but got %d", expect, got)
		}
	})
	t.Run("interval", func(t *testing.T) {
		maxElapsedTime := time.Second
		interval := time.Millisecond
		p := &RetryPolicy{
			Constant: &RetryPolicyConstant{
				MaxElapsedTime: (*Duration)(&maxElapsedTime),
				Interval:       (*Duration)(&interval),
				// MaxRetries: &maxRetries, // default value is 5
			},
		}
		_, cancel, b, err := p.Build(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer cancel()

		var i int
		_ = backoff.Retry(func() error {
			i++
			return errors.New("retry")
		}, b)
		if got, expect := i, 6; got != expect {
			t.Errorf("expect %d but got %d", expect, got)
		}
	})
	t.Run("max retires", func(t *testing.T) {
		maxElapsedTime := time.Second
		interval := time.Millisecond
		maxRetries := 1
		p := &RetryPolicy{
			Constant: &RetryPolicyConstant{
				MaxElapsedTime: (*Duration)(&maxElapsedTime),
				Interval:       (*Duration)(&interval),
				MaxRetries:     &maxRetries,
			},
		}
		_, cancel, b, err := p.Build(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer cancel()

		var i int
		_ = backoff.Retry(func() error {
			i++
			return errors.New("retry")
		}, b)
		if got, expect := i, 2; got != expect {
			t.Errorf("expect %d but got %d", expect, got)
		}
	})
}

func TestRetryPolicyExponential(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		p := &RetryPolicy{
			Exponential: &RetryPolicyExponential{},
		}
		_, cancel, b, err := p.Build(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer cancel()

		var i int
		_ = backoff.Retry(func() error {
			i++
			return errors.New("retry")
		}, b)
		if got, expect := i, 6; got != expect {
			t.Errorf("expect %d but got %d", expect, got)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		maxElapsedTime := time.Millisecond
		p := &RetryPolicy{
			Exponential: &RetryPolicyExponential{
				MaxElapsedTime: (*Duration)(&maxElapsedTime),
			},
		}
		_, cancel, b, err := p.Build(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer cancel()

		var i int
		_ = backoff.Retry(func() error {
			i++
			return errors.New("retry")
		}, b)
		if got, expect := i, 1; got != expect {
			t.Errorf("expect %d but got %d", expect, got)
		}
	})
	t.Run("interval", func(t *testing.T) {
		// 100ms+200ms+300ms+300ms+300ms > 1s => 4retries
		maxElapsedTime := time.Second
		initialInterval := 100 * time.Millisecond
		factor := 2.0
		jitterFactor := 0.0
		maxInterval := 300 * time.Millisecond
		p := &RetryPolicy{
			Exponential: &RetryPolicyExponential{
				MaxElapsedTime:  (*Duration)(&maxElapsedTime),
				InitialInterval: (*Duration)(&initialInterval),
				Factor:          &factor,
				JitterFactor:    &jitterFactor,
				MaxInterval:     (*Duration)(&maxInterval),
			},
		}
		_, cancel, b, err := p.Build(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer cancel()

		var i int
		_ = backoff.Retry(func() error {
			i++
			return errors.New("retry")
		}, b)
		if got, expect := i, 5; got != expect {
			t.Errorf("expect %d but got %d", expect, got)
		}
	})
	t.Run("max retires", func(t *testing.T) {
		maxElapsedTime := time.Second
		initialInterval := 100 * time.Millisecond
		factor := 2.0
		maxInterval := 300 * time.Millisecond
		maxRetries := 2
		p := &RetryPolicy{
			Exponential: &RetryPolicyExponential{
				MaxElapsedTime:  (*Duration)(&maxElapsedTime),
				InitialInterval: (*Duration)(&initialInterval),
				Factor:          &factor,
				MaxInterval:     (*Duration)(&maxInterval),
				MaxRetries:      &maxRetries,
			},
		}
		_, cancel, b, err := p.Build(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer cancel()

		var i int
		_ = backoff.Retry(func() error {
			i++
			return errors.New("retry")
		}, b)
		if got, expect := i, 3; got != expect {
			t.Errorf("expect %d but got %d", expect, got)
		}
	})
}

func TestRetryNoPolicy(t *testing.T) {
	p := &RetryPolicy{}
	_, cancel, b, err := p.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	var i int
	_ = backoff.Retry(func() error {
		i++
		return errors.New("retry")
	}, b)
	if got, expect := i, 1; got != expect {
		t.Errorf("expect %d but got %d", expect, got)
	}
}
