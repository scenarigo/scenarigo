package context_test

import (
	gocontext "context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
)

func TestContext(t *testing.T) {
	t.Run("ScenarioTitle", func(t *testing.T) {
		title := "test"
		ctx := context.FromT(t).WithScenarioTitle(title)
		if got := ctx.ScenarioTitle(); got != title {
			t.Errorf("expect %q but got %q", title, got)
		}
	})
	t.Run("ScenarioFilepath", func(t *testing.T) {
		path := "test.yaml"
		ctx := context.FromT(t).WithScenarioFilepath(path)
		if got := ctx.ScenarioFilepath(); got != path {
			t.Errorf("expect %q but got %q", path, got)
		}
	})
	t.Run("node", func(t *testing.T) {
		ctx := context.FromT(t)
		node := ast.String(token.String("", "", nil))
		ctx = ctx.WithNode(node)
		if ctx.Node() != node {
			t.Fatal("failed to get node")
		}
	})
}

func TestRunWithRetry(t *testing.T) {
	ctx := context.FromT(t)
	interval := schema.Duration(time.Millisecond)
	maxRetries := 1
	var (
		i        int
		canceled int32
	)
	f := func() { atomic.AddInt32(&canceled, 1) }
	context.RunWithRetry(ctx, "sub", func(ctx *context.Context) {
		i++
		go func() {
			<-ctx.RequestContext().Done()
			f()
		}()
		if i > 1 {
			return
		}
		ctx.Reporter().FailNow()
	}, &schema.RetryPolicyConstant{
		Interval:   &interval,
		MaxRetries: &maxRetries,
	})
	if atomic.LoadInt32(&canceled) == 0 {
		t.Errorf("context is not canceled")
	}
}

func TestTeardown_RequestContext(t *testing.T) {
	t.Run("RequestContext is not canceled during teardown", func(t *testing.T) {
		var teardownErr error
		reporter.Run(func(rptr reporter.Reporter) {
			ctx := context.New(rptr)
			ctx.Run("test", func(ctx *context.Context) {
				ctx.Teardown("teardown", func(ctx *context.Context) {
					if err := ctx.RequestContext().Err(); err != nil {
						teardownErr = err
					}
				})
			})
		})
		if teardownErr != nil {
			t.Errorf("RequestContext should not be canceled during teardown but got: %v", teardownErr)
		}
	})
	t.Run("RequestContext is not canceled during teardown after RunWithRetry", func(t *testing.T) {
		var teardownErr error
		reporter.Run(func(rptr reporter.Reporter) {
			ctx := context.New(rptr)
			context.RunWithRetry(ctx, "step", func(ctx *context.Context) {
				ctx.Teardown("teardown", func(ctx *context.Context) {
					if err := ctx.RequestContext().Err(); err != nil {
						teardownErr = err
					}
				})
			}, nil)
		})
		if teardownErr != nil {
			t.Errorf("RequestContext should not be canceled during teardown after RunWithRetry but got: %v", teardownErr)
		}
	})
	t.Run("RequestContext is not canceled during teardown even if parent context is canceled", func(t *testing.T) {
		var teardownErr error
		reporter.Run(func(rptr reporter.Reporter) {
			reqCtx, cancel := gocontext.WithCancel(gocontext.Background())
			ctx := context.New(rptr).WithRequestContext(reqCtx)
			ctx.Run("test", func(ctx *context.Context) {
				ctx.Teardown("teardown", func(ctx *context.Context) {
					if err := ctx.RequestContext().Err(); err != nil {
						teardownErr = err
					}
				})
				// Cancel the parent context (simulates SIGINT).
				cancel()
			})
		})
		if teardownErr != nil {
			t.Errorf("RequestContext should not be canceled during teardown even if parent is canceled but got: %v", teardownErr)
		}
	})
	t.Run("SIGINT simulation: step canceled but teardown still runs with valid RequestContext", func(t *testing.T) {
		var (
			stepCanceled  bool
			teardownRan   bool
			teardownErr   error
			teardownOrder []string
		)
		reporter.Run(func(rptr reporter.Reporter) {
			reqCtx, cancel := gocontext.WithCancel(gocontext.Background())
			defer cancel()
			ctx := context.New(rptr).WithRequestContext(reqCtx)

			// Simulate multiple steps via RunWithRetry, with SIGINT arriving during a step.
			for i := range 2 {
				stepName := fmt.Sprintf("step %d", i+1)
				context.RunWithRetry(ctx, stepName, func(ctx *context.Context) {
					ctx.Teardown(stepName, func(ctx *context.Context) {
						teardownOrder = append(teardownOrder, stepName)
						teardownRan = true
						if err := ctx.RequestContext().Err(); err != nil {
							teardownErr = err
						}
					})

					// Cancel during the last step (simulates SIGINT).
					if i == 1 {
						cancel()
						// Verify the step's RequestContext is now canceled.
						if err := ctx.RequestContext().Err(); err != nil {
							stepCanceled = true
						}
					}
				}, nil)
			}
		})
		if !stepCanceled {
			t.Error("step's RequestContext should be canceled after SIGINT")
		}
		if !teardownRan {
			t.Error("teardown should have run even after SIGINT")
		}
		if teardownErr != nil {
			t.Errorf("teardown's RequestContext should not be canceled but got: %v", teardownErr)
		}
		if len(teardownOrder) != 2 {
			t.Errorf("expected 2 teardowns but got %d", len(teardownOrder))
		}
	})
}
