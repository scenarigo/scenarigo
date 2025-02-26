package scenarigo

import (
	"bytes"
	"os"
	"testing"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
)

func TestRunScenario_Context_ScenarioFilepath(t *testing.T) {
	path := createTempScenario(t, `
steps:
  - ref: '{{plugins.getScenarioFilepath}}'
  `)
	sceanrios, err := schema.LoadScenarios(path)
	if err != nil {
		t.Fatalf("failed to load scenario: %s", err)
	}
	if len(sceanrios) != 1 {
		t.Fatalf("unexpected scenario length: %d", len(sceanrios))
	}

	var (
		got string
		log bytes.Buffer
	)
	ok := reporter.Run(func(rptr reporter.Reporter) {
		ctx := context.New(rptr).WithPlugins(map[string]interface{}{
			"getScenarioFilepath": plugin.StepFunc(func(ctx *context.Context, step *schema.Step) *context.Context {
				got = ctx.ScenarioFilepath()
				return ctx
			}),
		})
		RunScenario(ctx, sceanrios[0])
	}, reporter.WithWriter(&log))
	if !ok {
		t.Fatalf("scenario failed:\n%s", log.String())
	}
	if got != path {
		t.Errorf("invalid filepath: %q", got)
	}
}

func createTempScenario(t *testing.T, scenario string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %s", err)
	}
	defer f.Close()
	if _, err := f.WriteString(scenario); err != nil {
		t.Fatalf("failed to write scenario: %s", err)
	}
	return f.Name()
}

func TestExecuteIf(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			vars   map[string]any
			expr   string
			expect bool
		}{
			"empty": {
				expect: true,
			},
			"no vars": {
				expr:   "{{true}}",
				expect: true,
			},
			"with vars": {
				vars: map[string]any{
					"foo": true,
				},
				expr:   "{{vars.foo}}",
				expect: true,
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				ctx := context.FromT(t).WithVars(test.vars)
				got, err := executeIf(ctx, test.expr)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if expect := test.expect; got != expect {
					t.Errorf("expected %t but got %t", expect, got)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			expr        string
			expectError string
		}{
			"invalid template": {
				expr:        "{{",
				expectError: `failed to execute: failed to parse "{{": col 3: expected '}}', found 'EOF'`,
			},
			"not bool": {
				expr:        "{{1}}",
				expectError: "must be bool but got int64",
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				ctx := context.FromT(t)
				if _, err := executeIf(ctx, test.expr); err == nil {
					t.Fatal("no error")
				} else if got, expect := err.Error(), test.expectError; got != expect {
					t.Errorf("expected %q but got %q", expect, got)
				}
			})
		}
	})
}
