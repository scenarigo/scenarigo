// +build !race

package scenarigo

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/reporter"
)

func TestLoadPlugin(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		scenarioYAML := `
plugins:
  simple: simple.so
  `
		runner, err := NewRunner(
			WithScenariosFromReader(strings.NewReader(scenarioYAML)),
			WithPluginDir("test/e2e/testdata/gen/plugins"),
		)
		if err != nil {
			t.Fatalf("failed to create runner: %s", err)
		}
		var log bytes.Buffer
		ok := reporter.Run(func(rptr reporter.Reporter) {
			runner.Run(context.New(rptr))
		}, reporter.WithWriter(&log))
		if !ok {
			t.Fatalf("scenario failed:\n%s", log.String())
		}
	})
	t.Run("failure", func(t *testing.T) {
		scenarioYAML := `
plugins:
  simple: invalid.so
  `
		runner, err := NewRunner(
			WithScenariosFromReader(strings.NewReader(scenarioYAML)),
			WithPluginDir("test/e2e/testdata/gen/plugins"),
		)
		if err != nil {
			t.Fatalf("failed to create runner: %s", err)
		}
		var log bytes.Buffer
		ok := reporter.Run(func(rptr reporter.Reporter) {
			runner.Run(context.New(rptr))
		}, reporter.WithWriter(&log))
		if ok {
			t.Fatal("expected error")
		}
	})
}
