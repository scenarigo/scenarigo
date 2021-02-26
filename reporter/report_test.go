package reporter

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
)

func TestGenerateTestReport(t *testing.T) {
	t.Run("passed", func(t *testing.T) {
		tests := map[string]struct {
			f      func(r Reporter)
			expect *TestReport
		}{
			"all step passed": {
				f: func(r Reporter) {
					r.Run("file1.yaml", func(r Reporter) {
						r.Run("scenario1", func(r Reporter) {
							r.Run("step1", func(r Reporter) {
							})
						})
					})
				},
				expect: &TestReport{
					Result: TestResultPassed,
					Files: []ScenarioFileReport{
						{
							Name:   "file1.yaml",
							Result: TestResultPassed,
							Scenarios: []ScenarioReport{
								{
									Name:   "scenario1",
									File:   "file1.yaml",
									Result: TestResultPassed,
									Steps: []StepReport{
										{
											Name:   "step1",
											Result: TestResultPassed,
										},
									},
								},
							},
						},
					},
				},
			},
			"has sub steps (include)": {
				f: func(r Reporter) {
					r.Run("file1.yaml", func(r Reporter) {
						r.Run("scenario1", func(r Reporter) {
							r.Run("include base.yaml", func(r Reporter) {
								r.Run("base.yaml", func(r Reporter) {
									r.Run("base scenario", func(r Reporter) {
										r.Run("base step", func(r Reporter) {
											r.Log("base step")
										})
									})
								})
							})
						})
					})
				},
				expect: &TestReport{
					Result: TestResultPassed,
					Files: []ScenarioFileReport{
						{
							Name:   "file1.yaml",
							Result: TestResultPassed,
							Scenarios: []ScenarioReport{
								{
									Name:   "scenario1",
									File:   "file1.yaml",
									Result: TestResultPassed,
									Steps: []StepReport{
										{
											Name:   "include base.yaml",
											Result: TestResultPassed,
											SubSteps: []SubStepReport{
												{
													Name:   "base.yaml",
													Result: TestResultPassed,
													SubSteps: []SubStepReport{
														{
															Name:   "base scenario",
															Result: TestResultPassed,
															SubSteps: []SubStepReport{
																{
																	Name:   "base step",
																	Result: TestResultPassed,
																	Logs: ReportLogs{
																		Info: []string{
																			"base step",
																		},
																	},
																},
															},
														},
													},
												},
											},
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
			t.Run(name, func(t *testing.T) {
				t.Run("reporter", func(t *testing.T) {
					r := run(func(r Reporter) {
						r.(*reporter).durationMeasurer = &fixedDurationMeasurer{}
						test.f(r)
					}, WithWriter(&nopWriter{}))
					checkReport(t, r, test.expect)
				})
				t.Run("testReporter", func(t *testing.T) {
					r := FromT(t)
					r.(*testReporter).durationMeasurer = &fixedDurationMeasurer{}
					test.f(r)
					test.expect.Name = t.Name()
					checkReport(t, r, test.expect)
				})
			})
		}
	})
	t.Run("failed", func(t *testing.T) {
		skipMsg := "skip"
		tests := map[string]struct {
			f      func(r Reporter)
			expect *TestReport
		}{
			"step failed": {
				f: func(r Reporter) {
					r.Run("file1.yaml", func(r Reporter) {
						r.Run("scenario1", func(r Reporter) {
							r.Run("step1", func(r Reporter) {
								r.Log("log")
							})
							r.Run("step2", func(r Reporter) {
								r.Fatal("fatal")
							})
							r.Run("step3", func(r Reporter) {
								r.Skip(skipMsg)
							})
						})
					})
				},
				expect: &TestReport{
					Result: TestResultFailed,
					Files: []ScenarioFileReport{
						{
							Name:   "file1.yaml",
							Result: TestResultFailed,
							Scenarios: []ScenarioReport{
								{
									Name:   "scenario1",
									File:   "file1.yaml",
									Result: TestResultFailed,
									Steps: []StepReport{
										{
											Name:   "step1",
											Result: TestResultPassed,
											Logs: ReportLogs{
												Info: []string{
													"log",
												},
											},
										},
										{
											Name:   "step2",
											Result: TestResultFailed,
											Logs: ReportLogs{
												Error: []string{
													"fatal",
												},
											},
										},
										{
											Name:   "step3",
											Result: TestResultSkipped,
											Logs: ReportLogs{
												Skip: &skipMsg,
											},
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
			t.Run(name, func(t *testing.T) {
				t.Run("reporter", func(t *testing.T) {
					r := run(func(r Reporter) {
						r.(*reporter).durationMeasurer = &fixedDurationMeasurer{}
						test.f(r)
					}, WithWriter(&nopWriter{}))
					checkReport(t, r, test.expect)
				})
			})
		}
	})
	t.Run("error", func(t *testing.T) {
		t.Run("nil", func(t *testing.T) {
			if _, err := GenerateTestReport(nil); err == nil {
				t.Fatal("no error")
			}
		})
		t.Run("not root", func(t *testing.T) {
			Run(func(r Reporter) {
				r.Run("child", func(r Reporter) {
					if _, err := GenerateTestReport(r); err == nil {
						t.Fatal("no error")
					}
				})
			}, WithWriter(&nopWriter{}))
		})
	})
}

func checkReport(t *testing.T, r Reporter, expect *TestReport) {
	t.Helper()
	report, err := GenerateTestReport(r)
	if err != nil {
		t.Fatalf("failed to generate report: %s", err)
	}
	if diff := cmp.Diff(expect, report); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestTestResult(t *testing.T) {
	tests := []struct {
		result TestResult
		expect string
	}{
		{
			result: TestResultUndefined,
			expect: "undefined",
		},
		{
			result: TestResultPassed,
			expect: "passed",
		},
		{
			result: TestResultFailed,
			expect: "failed",
		},
		{
			result: TestResultSkipped,
			expect: "skipped",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.expect, func(t *testing.T) {
			t.Run("json", func(t *testing.T) {
				b, err := json.Marshal(test.result)
				if err != nil {
					t.Fatal(err)
				}
				if got, expect := string(b), fmt.Sprintf("%q", test.expect); got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
				var tr TestResult
				if err := json.Unmarshal(b, &tr); err != nil {
					t.Fatal(err)
				}
				if got, expect := tr.String(), test.result.String(); got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
			})
			t.Run("yaml", func(t *testing.T) {
				b, err := yaml.Marshal(test.result)
				if err != nil {
					t.Fatal(err)
				}
				if got, expect := strings.TrimRight(string(b), "\n"), test.expect; got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
				var tr TestResult
				if err := yaml.Unmarshal(b, &tr); err != nil {
					t.Fatal(err)
				}
				if got, expect := tr.String(), test.result.String(); got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
			})
		})
	}
}

func TestTestDuration(t *testing.T) {
	tests := []struct {
		duration TestDuration
		expect   string
	}{
		{
			duration: 445506789000000,
			expect:   "123h45m6.789s",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.expect, func(t *testing.T) {
			t.Run("json", func(t *testing.T) {
				b, err := json.Marshal(test.duration)
				if err != nil {
					t.Fatal(err)
				}
				if got, expect := string(b), fmt.Sprintf("%q", test.expect); got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
				var d TestDuration
				if err := json.Unmarshal(b, &d); err != nil {
					t.Fatal(err)
				}
				if got, expect := d, test.duration; got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
			})
			t.Run("yaml", func(t *testing.T) {
				b, err := yaml.Marshal(test.duration)
				if err != nil {
					t.Fatal(err)
				}
				if got, expect := strings.TrimRight(string(b), "\n"), test.expect; got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
				var d TestDuration
				if err := yaml.Unmarshal(b, &d); err != nil {
					t.Fatal(err)
				}
				if got, expect := d, test.duration; got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
			})
		})
	}
}

func TestTestReport_MarshalXML(t *testing.T) {
	skipMsg := "skip"
	tests := map[string]struct {
		report *TestReport
	}{
		"passed": {
			report: &TestReport{
				Result: TestResultFailed,
				Files: []ScenarioFileReport{
					{
						Name:     "file1.yaml",
						Result:   TestResultFailed,
						Duration: TestDuration(123 * time.Millisecond),
						Scenarios: []ScenarioReport{
							{
								Name:     "passed scenario",
								File:     "file1.yaml",
								Result:   TestResultPassed,
								Duration: TestDuration(100 * time.Millisecond),
								Steps: []StepReport{
									{
										Name:     "passed step",
										Result:   TestResultPassed,
										Duration: TestDuration(100 * time.Millisecond),
									},
								},
							},
							{
								Name:     "failed scenario",
								File:     "file1.yaml",
								Result:   TestResultFailed,
								Duration: TestDuration(23 * time.Millisecond),
								Steps: []StepReport{
									{
										Name:     "passed step",
										Result:   TestResultPassed,
										Duration: TestDuration(20 * time.Millisecond),
										Logs: ReportLogs{
											Info: []string{
												"info",
											},
										},
									},
									{
										Name:     "failed step",
										Result:   TestResultFailed,
										Duration: TestDuration(3 * time.Millisecond),
										Logs: ReportLogs{
											Info: []string{
												"info",
											},
											Error: []string{
												"error",
											},
										},
									},
									{
										Name:   "skipped step",
										Result: TestResultSkipped,
										Logs: ReportLogs{
											Skip: &skipMsg,
										},
									},
								},
							},
							{
								Name:   "skipped scenario",
								File:   "file1.yaml",
								Result: TestResultSkipped,
								Steps: []StepReport{
									{
										Name:   "skipped step",
										Result: TestResultSkipped,
										Logs: ReportLogs{
											Skip: &skipMsg,
										},
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
		t.Run(name, func(t *testing.T) {
			b, err := xml.MarshalIndent(test.report, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal: %s", err)
			}

			f, err := os.Open("testdata/report.xml")
			if err != nil {
				t.Fatalf("failed to open: %s", err)
			}
			defer f.Close()
			expected, err := ioutil.ReadAll(f)
			if err != nil {
				t.Fatalf("failed to read: %s", err)
			}

			if diff := cmp.Diff(
				strings.Trim(string(expected), "\n"),
				string(b),
			); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
