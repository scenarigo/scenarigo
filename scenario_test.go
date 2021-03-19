package scenarigo

import (
	"bytes"
	"errors"
	"io/ioutil"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/plugin"
	"github.com/zoncoen/scenarigo/protocol"
	"github.com/zoncoen/scenarigo/reporter"
	"github.com/zoncoen/scenarigo/schema"
)

func TestRunScenario(t *testing.T) {
	tests := map[string]struct {
		steps  []*schema.Step
		expect *reporter.TestReport
	}{
		"passed": {
			steps: []*schema.Step{
				{
					Request: schema.Request{
						Invoker: protocol.InvokerFunc(func(ctx *context.Context) (*context.Context, interface{}, error) {
							return ctx, nil, nil
						}),
					},
				},
			},
			expect: &reporter.TestReport{
				Result: reporter.TestResultPassed,
				Files: []reporter.ScenarioFileReport{
					{
						Result: reporter.TestResultPassed,
						Scenarios: []reporter.ScenarioReport{
							{
								Result: reporter.TestResultPassed,
								Steps: []reporter.StepReport{
									{
										Result: reporter.TestResultPassed,
									},
								},
							},
						},
					},
				},
			},
		},
		"request failed": {
			steps: []*schema.Step{
				{
					Title: "failed",
					Request: schema.Request{
						Invoker: protocol.InvokerFunc(func(ctx *context.Context) (*context.Context, interface{}, error) {
							return ctx, nil, errors.New("request failed")
						}),
					},
				},
				{
					Title: "skipped",
				},
			},
			expect: &reporter.TestReport{
				Result: reporter.TestResultFailed,
				Files: []reporter.ScenarioFileReport{
					{
						Result: reporter.TestResultFailed,
						Scenarios: []reporter.ScenarioReport{
							{
								Result: reporter.TestResultFailed,
								Steps: []reporter.StepReport{
									{
										Name:   "failed",
										Result: reporter.TestResultFailed,
									},
									{
										Name:   "skipped",
										Result: reporter.TestResultSkipped,
									},
								},
							},
						},
					},
				},
			},
		},
		"assertion failed": {
			steps: []*schema.Step{
				{
					Title: "failed",
					Request: schema.Request{
						Invoker: protocol.InvokerFunc(func(ctx *context.Context) (*context.Context, interface{}, error) {
							return ctx, nil, nil
						}),
					},
					Expect: schema.Expect{
						AssertionBuilder: protocol.AssertionBuilderFunc(func(ctx *context.Context) (assert.Assertion, error) {
							return assert.AssertionFunc(func(v interface{}) error {
								return errors.New("assertion error")
							}), nil
						}),
					},
				},
				{
					Title: "skipped",
				},
			},
			expect: &reporter.TestReport{
				Result: reporter.TestResultFailed,
				Files: []reporter.ScenarioFileReport{
					{
						Result: reporter.TestResultFailed,
						Scenarios: []reporter.ScenarioReport{
							{
								Result: reporter.TestResultFailed,
								Steps: []reporter.StepReport{
									{
										Name:   "failed",
										Result: reporter.TestResultFailed,
									},
									{
										Name:   "skipped",
										Result: reporter.TestResultSkipped,
									},
								},
							},
						},
					},
				},
			},
		},
		"included scenario failed": {
			steps: []*schema.Step{
				{
					Title:   "failed",
					Include: "testdata/base_error.yaml",
				},
				{
					Title: "skipped",
				},
			},
			expect: &reporter.TestReport{
				Result: reporter.TestResultFailed,
				Files: []reporter.ScenarioFileReport{
					{
						Result: reporter.TestResultFailed,
						Scenarios: []reporter.ScenarioReport{
							{
								Result: reporter.TestResultFailed,
								Steps: []reporter.StepReport{
									{
										Name:   "failed",
										Result: reporter.TestResultFailed,
										SubSteps: []reporter.SubStepReport{
											{
												Name:   "testdata/base_error.yaml",
												Result: reporter.TestResultFailed,
												SubSteps: []reporter.SubStepReport{
													{
														Name:   "POST /echo",
														Result: reporter.TestResultFailed,
													},
												},
											},
										},
									},
									{
										Name:   "skipped",
										Result: reporter.TestResultSkipped,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for name, test := range tests {
		test := test
		var report *reporter.TestReport
		t.Run(name, func(t *testing.T) {
			reporter.Run(func(r reporter.Reporter) {
				RunScenario(context.New(r), &schema.Scenario{
					Steps: test.steps,
				})

				var err error
				report, err = reporter.GenerateTestReport(r)
				if err != nil {
					t.Fatal(err)
				}
			})
		})
		if diff := cmp.Diff(
			test.expect,
			report,
			cmp.FilterValues(func(_, _ reporter.TestDuration) bool {
				return true
			}, cmp.Ignore()),
			cmp.FilterValues(func(_, _ reporter.ReportLogs) bool {
				return true
			}, cmp.Ignore()),
		); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestRunScenario_Error(t *testing.T) {
	t.Run("scenario is nil", func(t *testing.T) {
		reporter.Run(func(r reporter.Reporter) {
			ctx := RunScenario(context.New(r), nil)
			if !ctx.Reporter().Failed() {
				t.Error("test passed")
			}
		})
	})
}

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
