package template_test

import (
	"fmt"
	"strings"
	"testing"

	sccontext "github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/template"
)

// TestContextInjection tests context injection for non-variadic functions.
// Test matrix:
//   - First param is *Context: yes / no
//   - User passes context explicitly: yes / no
func TestContextInjection(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     func(t *testing.T) map[string]any
		expected string
	}{
		// First param is *Context
		{
			name:     "ctx param with auto injection",
			template: `{{vars.fn(vars.value)}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(c *sccontext.Context, v string) string {
						if c == nil {
							t.Fatal("context must not be nil")
						}
						return "ctx:" + v
					},
					"value": "value",
				}
			},
			expected: "ctx:value",
		},
		{
			name:     "ctx param with explicit context",
			template: `{{vars.fn(ctx, vars.value)}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(c *sccontext.Context, v string) string {
						if c == nil {
							t.Fatal("context must not be nil")
						}
						return "explicit:" + v
					},
					"value": "value",
				}
			},
			expected: "explicit:value",
		},
		// First param is NOT *Context (fast path)
		{
			name:     "no ctx param",
			template: `{{vars.fn("a", "b")}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(a, b string) string {
						return a + "+" + b
					},
				}
			},
			expected: "a+b",
		},
		{
			name:     "no ctx param single arg",
			template: `{{vars.fn("only")}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(v string) string {
						return "got:" + v
					},
				}
			},
			expected: "got:only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reporter.FromT(t)
			ctx := sccontext.New(r)
			ctx = ctx.WithVars(tt.vars(t))

			tmpl, err := template.New(tt.template)
			if err != nil {
				t.Fatalf("failed to create template: %s", err)
			}
			execCtx := template.WithExecutionContext(ctx.RequestContext(), ctx)
			got, err := tmpl.Execute(execCtx, ctx)
			if err != nil {
				t.Fatalf("failed to execute template: %s", err)
			}
			if got.(string) != tt.expected {
				t.Fatalf("expected %q but got %q", tt.expected, got)
			}
		})
	}
}

// TestContextInjectionWithNilArgs tests that nil arguments are correctly passed
// without being replaced by *Context. This is the bug reported in issue #690.
func TestContextInjectionWithNilArgs(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     func(t *testing.T) map[string]any
		expected string
	}{
		{
			name:     "nil arg to single param function",
			template: `{{vars.fn(vars.nilValue)}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(v any) string {
						if v != nil {
							t.Fatalf("expected nil but got %T: %v", v, v)
						}
						return "got nil"
					},
					"nilValue": nil,
				}
			},
			expected: "got nil",
		},
		{
			name:     "nil arg does not shift subsequent args",
			template: `{{vars.fn(vars.nilValue, "second")}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(a, b any) string {
						if a != nil {
							t.Fatalf("expected first arg to be nil but got %T: %v", a, a)
						}
						if b != "second" {
							t.Fatalf("expected second arg to be 'second' but got %T: %v", b, b)
						}
						return "ok"
					},
					"nilValue": nil,
				}
			},
			expected: "ok",
		},
		{
			name:     "nil arg with context param injects context",
			template: `{{vars.fn(vars.nilValue)}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(c *sccontext.Context, v any) string {
						if c == nil {
							t.Fatal("context must not be nil")
						}
						if v != nil {
							t.Fatalf("expected second arg to be nil but got %T: %v", v, v)
						}
						return "ctx+nil"
					},
					"nilValue": nil,
				}
			},
			expected: "ctx+nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reporter.FromT(t)
			ctx := sccontext.New(r)
			ctx = ctx.WithVars(tt.vars(t))

			tmpl, err := template.New(tt.template)
			if err != nil {
				t.Fatalf("failed to create template: %s", err)
			}
			execCtx := template.WithExecutionContext(ctx.RequestContext(), ctx)
			got, err := tmpl.Execute(execCtx, ctx)
			if err != nil {
				t.Fatalf("failed to execute template: %s", err)
			}
			if got.(string) != tt.expected {
				t.Fatalf("expected %q but got %q", tt.expected, got)
			}
		})
	}
}

// TestContextInjectionWithVariadicFunctions tests context injection for variadic functions.
// Test matrix:
//   - First param is *Context: yes / no
//   - User passes context explicitly: yes / no
//   - Number of variadic args: 0 / 1 / 2+
func TestContextInjectionWithVariadicFunctions(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     func(t *testing.T) map[string]any
		expected string
	}{
		// First param is *Context, auto injection
		{
			name:     "ctx param auto injection with 0 variadic args",
			template: `{{vars.fn()}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(c *sccontext.Context, args ...any) string {
						if c == nil {
							t.Fatal("context must not be nil")
						}
						return fmt.Sprintf("auto:%d", len(args))
					},
				}
			},
			expected: "auto:0",
		},
		{
			name:     "ctx param auto injection with 1 variadic arg",
			template: `{{vars.fn("a")}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(c *sccontext.Context, args ...any) string {
						if c == nil {
							t.Fatal("context must not be nil")
						}
						if len(args) != 1 || args[0] != "a" {
							t.Fatalf("unexpected args: %v", args)
						}
						return fmt.Sprintf("auto:%d", len(args))
					},
				}
			},
			expected: "auto:1",
		},
		{
			name:     "ctx param auto injection with 2+ variadic args",
			template: `{{vars.fn("a", "b", "c")}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(c *sccontext.Context, args ...any) string {
						if c == nil {
							t.Fatal("context must not be nil")
						}
						if len(args) != 3 {
							t.Fatalf("expected 3 args but got %d", len(args))
						}
						return fmt.Sprintf("auto:%d", len(args))
					},
				}
			},
			expected: "auto:3",
		},
		// First param is *Context, explicit context
		{
			name:     "ctx param explicit with 0 variadic args",
			template: `{{vars.fn(ctx)}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(c *sccontext.Context, args ...any) string {
						if c == nil {
							t.Fatal("context must not be nil")
						}
						return fmt.Sprintf("explicit:%d", len(args))
					},
				}
			},
			expected: "explicit:0",
		},
		{
			name:     "ctx param explicit with 1 variadic arg",
			template: `{{vars.fn(ctx, "a")}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(c *sccontext.Context, args ...any) string {
						if c == nil {
							t.Fatal("context must not be nil")
						}
						if len(args) != 1 || args[0] != "a" {
							t.Fatalf("unexpected args: %v", args)
						}
						return fmt.Sprintf("explicit:%d", len(args))
					},
				}
			},
			expected: "explicit:1",
		},
		{
			name:     "ctx param explicit with 2+ variadic args",
			template: `{{vars.fn(ctx, "a", "b")}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(c *sccontext.Context, args ...any) string {
						if c == nil {
							t.Fatal("context must not be nil")
						}
						if len(args) != 2 {
							t.Fatalf("expected 2 args but got %d", len(args))
						}
						return fmt.Sprintf("explicit:%d", len(args))
					},
				}
			},
			expected: "explicit:2",
		},
		// First param is NOT *Context (fast path)
		{
			name:     "no ctx param with 0 variadic args",
			template: `{{vars.fn()}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(args ...string) string {
						return fmt.Sprintf("no-ctx:%d", len(args))
					},
				}
			},
			expected: "no-ctx:0",
		},
		{
			name:     "no ctx param with 1 variadic arg",
			template: `{{vars.fn("a")}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(args ...string) string {
						if len(args) != 1 || args[0] != "a" {
							t.Fatalf("unexpected args: %v", args)
						}
						return fmt.Sprintf("no-ctx:%d", len(args))
					},
				}
			},
			expected: "no-ctx:1",
		},
		{
			name:     "no ctx param with 2+ variadic args",
			template: `{{vars.fn("a", "b", "c")}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(args ...string) string {
						if len(args) != 3 {
							t.Fatalf("expected 3 args but got %d", len(args))
						}
						return strings.Join(args, ",")
					},
				}
			},
			expected: "a,b,c",
		},
		// Mixed: fixed param (not ctx) + variadic
		{
			name:     "fixed non-ctx param with variadic args",
			template: `{{vars.fn("prefix", "a", "b")}}`,
			vars: func(t *testing.T) map[string]any {
				return map[string]any{
					"fn": func(prefix string, args ...string) string {
						return prefix + ":" + strings.Join(args, ",")
					},
				}
			},
			expected: "prefix:a,b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reporter.FromT(t)
			ctx := sccontext.New(r)
			ctx = ctx.WithVars(tt.vars(t))

			tmpl, err := template.New(tt.template)
			if err != nil {
				t.Fatalf("failed to create template: %s", err)
			}
			execCtx := template.WithExecutionContext(ctx.RequestContext(), ctx)
			got, err := tmpl.Execute(execCtx, ctx)
			if err != nil {
				t.Fatalf("failed to execute template: %s", err)
			}
			if got.(string) != tt.expected {
				t.Fatalf("expected %q but got %q", tt.expected, got)
			}
		})
	}
}

// TestContextInjectionArgumentErrors tests error cases for argument count validation
// when context injection is involved.
func TestContextInjectionArgumentErrors(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		vars        map[string]any
		expectError string
	}{
		// Non-variadic with ctx param
		{
			name:     "ctx param with too many args",
			template: `{{vars.fn("a", "b")}}`,
			vars: map[string]any{
				"fn": func(c *sccontext.Context, v string) string { return v },
			},
			// When user provides 2 args for func(*Context, string), no injection occurs
			// and the first arg "a" fails to be assigned to *Context
			expectError: "can't use string as *context.Context",
		},
		{
			name:     "ctx param with too few args",
			template: `{{vars.fn()}}`,
			vars: map[string]any{
				"fn": func(c *sccontext.Context, a, b string) string { return a + b },
			},
			// Context is auto-injected, so user needs to provide 2 args for (a, b string)
			expectError: "expected minimum argument number is 2. but specified 0 arguments",
		},
		// Variadic with ctx param
		{
			name:     "ctx param variadic with too few args for fixed params",
			template: `{{vars.fn()}}`,
			vars: map[string]any{
				"fn": func(c *sccontext.Context, required string, args ...string) string { return required },
			},
			expectError: "too few arguments to function: expected minimum argument number is 1. but specified 0 arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reporter.FromT(t)
			ctx := sccontext.New(r)
			ctx = ctx.WithVars(tt.vars)

			tmpl, err := template.New(tt.template)
			if err != nil {
				t.Fatalf("failed to create template: %s", err)
			}
			execCtx := template.WithExecutionContext(ctx.RequestContext(), ctx)
			_, err = tmpl.Execute(execCtx, ctx)
			if err == nil {
				t.Fatal("expected error but got nil")
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Fatalf("expected error containing %q but got %q", tt.expectError, err.Error())
			}
		})
	}
}
