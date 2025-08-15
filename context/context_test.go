package context_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/schema"
)

func TestContext(t *testing.T) {
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
