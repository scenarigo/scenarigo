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
	t.Run("enabledColor", func(t *testing.T) {
		ctx := context.FromT(t)
		ctx = ctx.WithEnabledColor(true)
		if !ctx.EnabledColor() {
			t.Fatal("failed to get enabledColor")
		}
	})
	t.Run("currentStep", func(t *testing.T) {
		ctx := context.FromT(t)

		// Test that CurrentStep returns nil initially
		if ctx.CurrentStep() != nil {
			t.Fatal("expected CurrentStep to be nil initially")
		}

		// Create a test CurrentStep
		currentStep := &context.CurrentStep{
			Index:       1,
			ID:          "test-step",
			Title:       "Test Step",
			Description: "This is a test step",
			Protocol:    "http",
			Request: map[string]interface{}{
				"method": "GET",
				"url":    "https://example.com",
			},
		}

		// Test WithCurrentStep
		ctx = ctx.WithCurrentStep(currentStep)

		// Test that CurrentStep returns the correct value
		got := ctx.CurrentStep()
		if got == nil {
			t.Fatal("expected CurrentStep to return non-nil value")
		}
		if got.Index != currentStep.Index {
			t.Errorf("expected Index %d but got %d", currentStep.Index, got.Index)
		}
		if got.ID != currentStep.ID {
			t.Errorf("expected ID %q but got %q", currentStep.ID, got.ID)
		}
		if got.Title != currentStep.Title {
			t.Errorf("expected Title %q but got %q", currentStep.Title, got.Title)
		}
		if got.Description != currentStep.Description {
			t.Errorf("expected Description %q but got %q", currentStep.Description, got.Description)
		}
		if got.Protocol != currentStep.Protocol {
			t.Errorf("expected Protocol %q but got %q", currentStep.Protocol, got.Protocol)
		}

		// Test that WithCurrentStep with nil sets CurrentStep to nil
		ctx2 := ctx.WithCurrentStep(nil)
		if ctx2.CurrentStep() != nil {
			t.Fatal("expected WithCurrentStep(nil) to set CurrentStep to nil")
		}

		// Test that the original context is not modified
		if ctx.CurrentStep() != currentStep {
			t.Fatal("expected original context to remain unchanged")
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
