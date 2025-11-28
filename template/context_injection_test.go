package template_test

import (
	"testing"

	sccontext "github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/template"
)

func TestExecuteFuncCallAutoContext(t *testing.T) {
	r := reporter.FromT(t)
	ctx := sccontext.New(r)
	ctx = ctx.WithVars(map[string]any{
		"fn": func(c *sccontext.Context, v string) string {
			if c == nil {
				t.Fatal("context must not be nil")
			}
			if c != ctx {
				t.Fatalf("expected context pointer %p but got %p", ctx, c)
			}
			return "ctx:" + v
		},
		"value": "value",
	})

	tmpl, err := template.New(`{{vars.fn(vars.value)}}`)
	if err != nil {
		t.Fatalf("failed to create template: %s", err)
	}
	execCtx := template.WithExecutionContext(ctx.RequestContext(), ctx)
	got, err := tmpl.Execute(execCtx, ctx)
	if err != nil {
		t.Fatalf("failed to execute template: %s", err)
	}
	if got.(string) != "ctx:value" {
		t.Fatalf("unexpected result: %v", got)
	}
}

func TestExecuteFuncCallExplicitContext(t *testing.T) {
	r := reporter.FromT(t)
	ctx := sccontext.New(r)
	ctx = ctx.WithVars(map[string]any{
		"fn": func(c *sccontext.Context, v string) string {
			if c == nil {
				t.Fatal("context must not be nil")
			}
			return "explicit:" + v
		},
		"value": "value",
	})

	tmpl, err := template.New(`{{vars.fn(ctx, vars.value)}}`)
	if err != nil {
		t.Fatalf("failed to create template: %s", err)
	}
	execCtx := template.WithExecutionContext(ctx.RequestContext(), ctx)
	got, err := tmpl.Execute(execCtx, ctx)
	if err != nil {
		t.Fatalf("failed to execute template: %s", err)
	}
	if got.(string) != "explicit:value" {
		t.Fatalf("unexpected result: %v", got)
	}
}
