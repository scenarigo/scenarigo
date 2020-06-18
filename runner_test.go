package scenarigo

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/protocol"
	"github.com/zoncoen/scenarigo/reporter"
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

type testProtocol struct {
	name    string
	invoker invoker
	builder builder
}

func (p *testProtocol) Name() string { return p.name }

func (p *testProtocol) UnmarshalRequest(_ []byte) (protocol.Invoker, error) {
	return p.invoker, nil
}

func (p *testProtocol) UnmarshalExpect(_ []byte) (protocol.AssertionBuilder, error) {
	return p.builder, nil
}

type invoker func(*context.Context) (*context.Context, interface{}, error)

func (f invoker) Invoke(ctx *context.Context) (*context.Context, interface{}, error) {
	return f(ctx)
}

type builder func(*context.Context) (assert.Assertion, error)

func (f builder) Build(ctx *context.Context) (assert.Assertion, error) {
	return f(ctx)
}

func TestRunnerWithDryRun(t *testing.T) {
	scenariosPath := filepath.Join("test", "e2e", "testdata", "scenarios")
	pluginPath := filepath.Join("test", "e2e", "testdata", "gen", "plugins")

	p := &testProtocol{
		name: "test",
		invoker: invoker(func(ctx *context.Context) (*context.Context, interface{}, error) {
			return ctx, nil, nil
		}),
		builder: builder(func(ctx *context.Context) (assert.Assertion, error) {
			return assert.AssertionFunc(func(_ interface{}) error { return nil }), nil
		}),
	}
	protocol.Register(p)
	defer protocol.Unregister(p.Name())

	runner, err := NewRunner(
		WithScenarios(scenariosPath),
		WithPluginDir(pluginPath),
		WithDryRun(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	var subtests []string
	var b bytes.Buffer
	ok := reporter.Run(func(rptr reporter.Reporter) {
		ctx := context.New(rptr)
		runner.Run(ctx)
		subtests = ctx.SubTests()
	}, reporter.WithWriter(&b))
	if !ok {
		t.Fatal(b.String())
	}
	if len(subtests) == 0 {
		t.Fatal("failed to capture subtests")
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

func TestRunner(t *testing.T) {
	tests := map[string]struct {
		path  string
		setup func(*testing.T) func()
	}{
		"run step with include": {
			path: filepath.Join("testdata", "use_include.yaml"),
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
	}
	for _, test := range tests {
		teardown := test.setup(t)
		defer teardown()
		runner, err := NewRunner(WithScenarios(test.path))
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
	}
	for _, test := range tests {
		teardown := test.setup(t)
		defer teardown()
		runner, err := NewRunner(WithScenarios(test.path))
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
