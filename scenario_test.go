package scenarigo

import (
	"errors"
	"testing"

	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/reporter"
	"github.com/zoncoen/scenarigo/schema"
)

func TestRunScenario_FailFast(t *testing.T) {
	var called bool
	scenario := &schema.Scenario{
		Steps: []*schema.Step{
			{
				Request: schema.Request{
					Invoker: invoker(func(ctx *context.Context) (*context.Context, interface{}, error) {
						return ctx, nil, nil
					}),
				},
				Expect: schema.Expect{
					AssertionBuilder: builder(func(ctx *context.Context) (assert.Assertion, error) {
						return nil, errors.New("failed")
					}),
				},
			},
			{
				Request: schema.Request{
					Invoker: invoker(func(ctx *context.Context) (*context.Context, interface{}, error) {
						called = true
						return ctx, nil, nil
					}),
				},
				Expect: schema.Expect{
					AssertionBuilder: builder(func(ctx *context.Context) (assert.Assertion, error) {
						return assert.AssertionFunc(func(_ interface{}) error { return nil }), nil
					}),
				},
			},
		},
	}
	reporter.Run(func(rptr reporter.Reporter) {
		runScenario(context.New(rptr), scenario)
	})
	if called {
		t.Fatal("following steps should be skipped if the previous step failed")
	}
}
