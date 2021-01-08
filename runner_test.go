package scenarigo

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/plugin"
	"github.com/zoncoen/scenarigo/reporter"
	"github.com/zoncoen/scenarigo/schema"
)

func TestRunnerWithScenarios(t *testing.T) {
	scenariosPath := filepath.Join("test", "e2e", "testdata", "scenarios")
	runner, err := NewRunner(WithScenarios(scenariosPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.scenarioFiles) == 0 {
		t.Fatal("failed to set scenario files")
	}
	for _, file := range runner.scenarioFiles {
		if !yamlPattern.MatchString(file) {
			t.Fatalf("invalid scenario file: %s", file)
		}
	}
}

func TestRunnerWithOptionsFromEnv(t *testing.T) {
	if err := os.Setenv(envScenarigoColor, "true"); err != nil {
		t.Fatalf("%+v", err)
	}
	defer os.Unsetenv(envScenarigoColor)
	runner, err := NewRunner(
		WithOptionsFromEnv(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !runner.enabledColor {
		t.Fatalf("failed to set enabledColor from env")
	}
}

type structFoo struct{}

func (*structFoo) Run(ctx *context.Context, step *schema.Step) *context.Context {
	vars, err := ctx.ExecuteTemplate(map[string]interface{}{
		"foo": "bar",
	})
	if err != nil {
		ctx.Reporter().Fatalf("invalid vars: %s", err)
	}
	return ctx.WithVars(vars)
}

type structPrint struct{}

func (*structPrint) Run(ctx *context.Context, step *schema.Step) *context.Context {
	return ctx
}

func TestRunner(t *testing.T) {
	tests := map[string]struct {
		path        string
		yaml        string
		setup       func(*testing.T) func()
		defaultVars map[string]interface{}
	}{
		"run step with include": {
			path: filepath.Join("testdata", "use_include.yaml"),
			setup: func(t *testing.T) func() {
				t.Helper()

				mux := http.NewServeMux()
				mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
					defer r.Body.Close()
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.Copy(w, r.Body)
				})

				s := httptest.NewServer(mux)
				if err := os.Setenv("TEST_ADDR", s.URL); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				return func() {
					s.Close()
					os.Unsetenv("TEST_ADDR")
				}
			},
		},
		"run with yaml": {
			yaml: `
---
title: /echo
steps:
- title: POST /echo
  protocol: http
  request:
    method: POST
    url: "{{env.TEST_ADDR}}/echo"
    body:
      message: "hello"
  expect:
    code: 200
    body:
      message: "hello"
`,
			setup: func(t *testing.T) func() {
				t.Helper()

				mux := http.NewServeMux()
				mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
					defer r.Body.Close()
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.Copy(w, r.Body)
				})

				s := httptest.NewServer(mux)
				if err := os.Setenv("TEST_ADDR", s.URL); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				return func() {
					s.Close()
					os.Unsetenv("TEST_ADDR")
				}
			},
		},
		"run with yaml ( default vars )": {
			yaml: `
---
title: default vars
steps:
- title: call foo
  ref: '{{ vars.Foo(ctx) }}'
  bind:
    vars:
      foo: '{{ vars.foo }}'
- title: print
  ref: '{{ vars.Print(vars.foo) }}'
`,
			setup: func(t *testing.T) func() {
				return func() {}
			},
			defaultVars: map[string]interface{}{
				"Foo": func(ctx *context.Context) plugin.Step {
					return new(structFoo)
				},
				"Print": func(args ...interface{}) plugin.Step {
					if len(args) != 1 {
						t.Fatalf("invalid argument number for print: %d", len(args))
					}
					arg, ok := args[0].(string)
					if !ok {
						t.Fatalf("invalid argument type. expected string type but got %T", args[0])
					}
					t.Log(arg)
					return new(structPrint)
				},
			},
		},
	}
	for _, test := range tests {
		teardown := test.setup(t)
		defer teardown()

		var opts []func(*Runner) error
		if test.path != "" {
			opts = append(opts, WithScenarios(test.path))
		}
		if test.yaml != "" {
			opts = append(opts, WithScenariosFromReader(strings.NewReader(test.yaml)))
		}
		if test.defaultVars != nil {
			opts = append(opts, WithDefaultVars(test.defaultVars))
		}
		runner, err := NewRunner(opts...)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		ok := reporter.Run(func(rptr reporter.Reporter) {
			runner.Run(context.New(rptr))
		}, reporter.WithWriter(&b))
		if !ok {
			t.Fatalf("scenario failed:\n%s", b.String())
		}
	}
}

func TestRunnerFail(t *testing.T) {
	tests := map[string]struct {
		path  string
		yaml  string
		setup func(*testing.T) func()
	}{
		"include invalid yaml": {
			path: filepath.Join("testdata", "use_include_error.yaml"),
			setup: func(t *testing.T) func() {
				t.Helper()

				mux := http.NewServeMux()
				mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					defer r.Body.Close()
					w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
					_, _ = io.Copy(w, r.Body)
				})

				s := httptest.NewServer(mux)
				if err := os.Setenv("TEST_ADDR", s.URL); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}

				return func() {
					s.Close()
					os.Unsetenv("TEST_ADDR")
				}
			},
		},
		"run with yaml": {
			yaml:  `invalid: value`,
			setup: func(t *testing.T) func() { return func() {} },
		},
	}
	for _, test := range tests {
		teardown := test.setup(t)
		defer teardown()

		var opts []func(*Runner) error
		if test.path != "" {
			opts = append(opts, WithScenarios(test.path))
		}
		if test.yaml != "" {
			opts = append(opts, WithScenariosFromReader(strings.NewReader(test.yaml)))
		}
		runner, err := NewRunner(opts...)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		ok := reporter.Run(func(rptr reporter.Reporter) {
			runner.Run(context.New(rptr))
		}, reporter.WithWriter(&b))
		if ok {
			t.Fatal("expected error but no error")
		}
	}
}

func TestRunner_ScenarioFiles(t *testing.T) {
	scenariosPath := filepath.Join("test", "e2e", "testdata", "scenarios")
	runner, err := NewRunner(WithScenarios(scenariosPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.ScenarioFiles()) == 0 {
		t.Fatal("failed to get scenario files")
	}
}

func TestRunner_ScenarioMap(t *testing.T) {
	runner, err := NewRunner(WithScenarios("testdata"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range runner.ScenarioFiles() {
		m, err := runner.ScenarioMap(context.FromT(t), file)
		if err != nil {
			t.Fatal(err)
		}
		if len(m) == 0 {
			t.Fatal("failed to get scenarios")
		}
		for _, steps := range m {
			if len(steps) == 0 {
				t.Fatal("failed to get steps from scenario map")
			}
		}
	}
}
