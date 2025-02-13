package template

import (
	"context"
	"sync"

	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/queryutil"
)

// Lazy represents a value with lazy initialization.
// You can create a Lazy value by using $.
type Lazy func(any) (any, error)

func (t *Template) executeLazyTemplate(ctx context.Context, data any) (any, error) {
	wc, done := t.executeTemplate(ctx, data)
	select {
	case result := <-done:
		return result.v, result.err
	case <-wc.blocked():
		// Delay template evaluation because the actual value is required.
		var once sync.Once
		return Lazy(func(v any) (any, error) {
			var c *waitContext
			once.Do(func() {
				// already executing
				c = wc
			})
			if c == nil {
				// re-execution is required from the second time onwards
				c, done = t.executeTemplate(ctx, data)
			}
			if err := c.set(v); err != nil {
				return nil, err
			}
			result := <-done
			return result.v, result.err
		}), nil
	}
}

func (t *Template) executeTemplate(ctx context.Context, data any) (*waitContext, chan templateResult) {
	wc := newWaitContext(ctx, data)
	done := make(chan templateResult)
	go func() {
		v, err := t.execute(ctx, wc)
		done <- templateResult{
			v:   v,
			err: err,
		}
	}()
	return wc, done
}

type templateResult struct {
	v   any
	err error
}

type waitContext struct {
	any                // base data
	extractActualValue func() (any, bool)
	ready              chan any
	blocked            func() <-chan struct{}
	setOnce            sync.Once
}

func newWaitContext(ctx context.Context, base any) *waitContext {
	block, cancel := context.WithCancel(context.Background())
	ready := make(chan any, 1)
	//nolint:exhaustruct
	return &waitContext{
		any: base,
		extractActualValue: sync.OnceValues(func() (any, bool) {
			cancel()
			select {
			case v := <-ready:
				return v, true
			case <-ctx.Done():
				// ignore canceled if the value is already set
				select {
				case v := <-ready:
					return v, true
				default:
				}
				return nil, false
			}
		}),
		ready:   ready,
		blocked: block.Done, //nolint:contextcheck
	}
}

func (c *waitContext) set(v any) error {
	var first bool
	c.setOnce.Do(func() {
		first = true
		c.ready <- v
	})
	if first {
		return nil
	}
	return errors.New("set an actual value twice")
}

// ExtractByKey implements query.KeyExtractor interface.
func (c *waitContext) ExtractByKey(key string) (any, bool) {
	if key == "$" {
		return c.extractActualValue()
	}
	k := queryutil.New().Key(key)
	res, err := k.Extract(c.any)
	if err != nil {
		return nil, false
	}
	return res, true
}
