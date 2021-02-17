package scenarigo

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/plugin"
	"github.com/zoncoen/scenarigo/reporter"
	"github.com/zoncoen/scenarigo/schema"
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
	f, err := ioutil.TempFile("", "*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %s", err)
	}
	defer f.Close()
	if _, err := f.WriteString(scenario); err != nil {
		t.Fatalf("failed to write scenario: %s", err)
	}
	return f.Name()
}
