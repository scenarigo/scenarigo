package reporter

import (
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func Test_testSummaryAppend(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		testSummary     testSummary
		testFileRelPath string
		reportFunc      func(r *reporter)
		expect          testSummary
	}{
		"passed": {
			testSummary: testSummary{
				mu:      sync.Mutex{},
				passed:  []string{},
				failed:  []string{},
				skipped: []string{},
			},
			testFileRelPath: "scenario/test.yaml",
			reportFunc:      func(r *reporter) {},
			expect: testSummary{
				mu:      sync.Mutex{},
				passed:  []string{"scenario/test.yaml"},
				failed:  []string{},
				skipped: []string{},
			},
		},
		"failed": {
			testSummary: testSummary{
				mu:      sync.Mutex{},
				passed:  []string{},
				failed:  []string{},
				skipped: []string{},
			},
			testFileRelPath: "scenario/test.yaml",
			reportFunc:      func(r *reporter) { r.Fail() },
			expect: testSummary{
				mu:      sync.Mutex{},
				passed:  []string{},
				failed:  []string{"scenario/test.yaml"},
				skipped: []string{},
			},
		},
		"skipped": {
			testSummary: testSummary{
				mu:      sync.Mutex{},
				passed:  []string{},
				failed:  []string{},
				skipped: []string{},
			},
			testFileRelPath: "scenario/test.yaml",
			reportFunc:      func(r *reporter) { r.skipped = 1 },
			expect: testSummary{
				mu:      sync.Mutex{},
				passed:  []string{},
				failed:  []string{},
				skipped: []string{"scenario/test.yaml"},
			},
		},
	}

	for name, test := range tests {
		tt := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := newReporter()
			tt.reportFunc(r)
			tt.testSummary.append(tt.testFileRelPath, r)

			if diff := cmp.Diff(tt.expect, tt.testSummary,
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
		testSummary testSummary
		expect      string
	}{
		"no failed test": {
			testSummary: testSummary{
				mu:      sync.Mutex{},
				passed:  []string{"scenario/test1.yaml", "scenario/test2.yaml"},
				failed:  []string{},
				skipped: []string{"scenario/test3.yaml"},
			},
			expect: `
3 tests run: 2 passed, 0 failed, 1 skipped

`,
		},
		"some tests failed": {
			testSummary: testSummary{
				mu:      sync.Mutex{},
				passed:  []string{"scenario/test1.yaml"},
				failed:  []string{"scenario/test2.yaml", "scenario/test3.yaml"},
				skipped: []string{"scenario/test4.yaml"},
			},
			expect: `
4 tests run: 1 passed, 2 failed, 1 skipped

Failed tests:
	- scenario/test2.yaml
	- scenario/test3.yaml

`,
		},
	}

	for name, test := range tests {
		tt := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := tt.testSummary.String(true)
			if diff := cmp.Diff(tt.expect, got); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_testSummaryFailedFiles(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		testSummary testSummary
		expect      string
	}{
		"no test failed": {
			testSummary: testSummary{
				mu:      sync.Mutex{},
				passed:  []string{"scenario/test1.yaml", "scenario/test2.yaml"},
				failed:  []string{},
				skipped: []string{},
			},
			expect: ``,
		},
		"some tests failed": {
			testSummary: testSummary{
				mu:      sync.Mutex{},
				passed:  []string{},
				failed:  []string{"scenario/test1.yaml", "scenario/test2.yaml"},
				skipped: []string{},
			},
			expect: strings.TrimPrefix(`
Failed tests:
	- scenario/test1.yaml
	- scenario/test2.yaml

`, "\n"),
		},
	}

	for name, test := range tests {
		tt := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := tt.testSummary.failedFiles()
			if diff := cmp.Diff(tt.expect, got); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}
