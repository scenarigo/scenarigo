package context

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zoncoen/query-go"
	"github.com/zoncoen/scenarigo/reporter"
)

func TestContext_ExtractKey(t *testing.T) {
	if err := os.Setenv("TEST_PORT", "5000"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	defer os.Unsetenv("TEST_PORT")

	vars := map[string]string{
		"foo": "bar",
	}
	tests := map[string]struct {
		ctx    func(*Context) *Context
		query  string
		expect interface{}
	}{
		"plugins": {
			ctx: func(ctx *Context) *Context {
				return ctx.WithPlugins(map[string]interface{}{
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
		"request": {
			ctx: func(ctx *Context) *Context {
				return ctx.WithRequest(vars)
			},
			query:  "request.foo",
			expect: "bar",
		},
		"response": {
			ctx: func(ctx *Context) *Context {
				return ctx.WithResponse(vars, nil, nil)
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
		test := test
		t.Run(name, func(t *testing.T) {
			ctx := New(reporter.FromT(t))
			if test.ctx != nil {
				ctx = test.ctx(ctx)
			}
			q, err := query.ParseString(test.query)
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
