package context

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/zoncoen/query-go"
)

func TestContext_ExtractKey(t *testing.T) {
	t.Setenv("TEST_PORT", "5000")

	vars := map[string]string{
		"foo": "bar",
	}
	tests := map[string]struct {
		ctx    func(*Context) *Context
		query  string
		expect any
	}{
		"plugins": {
			ctx: func(ctx *Context) *Context {
				return ctx.WithPlugins(map[string]any{
					"key": "value",
				})
			},
			query:  "plugins.key",
			expect: "value",
		},
		"vars": {
			ctx: func(ctx *Context) *Context {
				return ctx.WithVars(vars)
			},
			query:  "vars.foo",
			expect: "bar",
		},
		"secrets": {
			ctx: func(ctx *Context) *Context {
				return ctx.WithSecrets(vars)
			},
			query:  "secrets.foo",
			expect: "bar",
		},
		"steps": {
			ctx: func(ctx *Context) *Context {
				steps := NewSteps()
				steps.Add("foo", &Step{
					Result: "passed",
				})
				return ctx.WithSteps(steps)
			},
			query:  "steps.foo.result",
			expect: "passed",
		},
		"request": {
			ctx: func(ctx *Context) *Context {
				return ctx.WithRequest(vars)
			},
			query:  "request.foo",
			expect: "bar",
		},
		"response": {
			ctx: func(ctx *Context) *Context {
				return ctx.WithResponse(vars)
			},
			query:  "response.foo",
			expect: "bar",
		},
		"env": {
			query:  "env.TEST_PORT",
			expect: "5000",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := New(reporter.FromT(t))
			if test.ctx != nil {
				ctx = test.ctx(ctx)
			}
			q, err := query.ParseString(test.query, queryutil.Options()...)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			got, err := q.Extract(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
		})
	}
}
