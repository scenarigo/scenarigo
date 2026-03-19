package reporter

import (
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/scenarigo/scenarigo/color"
)

func Test_testSummaryAppend(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		testSummary     *testSummary
		testFileRelPath string
		reportFunc      func(r *reporter)
		expect          *testSummary
	}{
		"passed": {
			testSummary: &testSummary{
				mu:           sync.Mutex{},
				passedCount:  0,
				failed:       []string{},
				skippedCount: 0,
			},
			testFileRelPath: "scenario/test.yaml",
			reportFunc:      func(r *reporter) {},
			expect: &testSummary{
				mu:           sync.Mutex{},
				passedCount:  1,
				failed:       []string{},
				skippedCount: 0,
			},
		},
		"failed": {
			testSummary: &testSummary{
				mu:           sync.Mutex{},
				passedCount:  0,
				failed:       []string{},
				skippedCount: 0,
			},
			testFileRelPath: "scenario/test.yaml",
			reportFunc:      func(r *reporter) { r.Fail() },
			expect: &testSummary{
				mu:           sync.Mutex{},
				passedCount:  0,
				failed:       []string{"scenario/test.yaml"},
				skippedCount: 0,
			},
		},
		"skipped": {
			testSummary: &testSummary{
				mu:           sync.Mutex{},
				passedCount:  0,
				failed:       []string{},
				skippedCount: 0,
			},
			testFileRelPath: "scenario/test.yaml",
			reportFunc:      func(r *reporter) { r.skipped = 1 },
			expect: &testSummary{
				mu:           sync.Mutex{},
				passedCount:  0,
				failed:       []string{},
				skippedCount: 1,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := newReporter()
			test.reportFunc(r)
			test.testSummary.append(test.testFileRelPath, r)

			if diff := cmp.Diff(test.expect, test.testSummary,
				cmpopts.IgnoreFields(testSummary{}, "mu"),
				cmp.AllowUnexported(testSummary{}),
			); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_testSummaryString(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		testSummary *testSummary
		expect      string
	}{
		"no failed test": {
			testSummary: &testSummary{
				mu:           sync.Mutex{},
				passedCount:  2,
				failed:       []string{},
				skippedCount: 1,
			},
			expect: `
3 tests run: 2 passed, 0 failed, 1 skipped

`,
		},
		"some tests failed": {
			testSummary: &testSummary{
				mu:           sync.Mutex{},
				passedCount:  1,
				failed:       []string{"scenario/test1.yaml", "scenario/test2.yaml"},
				skippedCount: 1,
			},
			expect: `
4 tests run: 1 passed, 2 failed, 1 skipped

Failed tests:
	- scenario/test1.yaml
	- scenario/test2.yaml

`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Create a ColorConfig with color disabled for testing
			colorConfig := color.New()
			colorConfig.SetEnabled(false)
			got := test.testSummary.String(colorConfig)
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_testSummaryFailedFiles(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		testSummary *testSummary
		expect      string
	}{
		"no test failed": {
			testSummary: &testSummary{
				mu:           sync.Mutex{},
				passedCount:  2,
				failed:       []string{},
				skippedCount: 0,
			},
			expect: ``,
		},
		"some tests failed": {
			testSummary: &testSummary{
				mu:           sync.Mutex{},
				passedCount:  0,
				failed:       []string{"scenario/test1.yaml", "scenario/test2.yaml"},
				skippedCount: 0,
			},
			expect: strings.TrimPrefix(`
Failed tests:
	- scenario/test1.yaml
	- scenario/test2.yaml

`, "\n"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := test.testSummary.failedFiles()
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}
