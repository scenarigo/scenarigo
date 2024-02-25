package scenarigo

import (
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/zoncoen/scenarigo/schema"
)

func Test_testSummaryAdd(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		testSummary      testSummary
		testFileRelPath  string
		testResultString string
		expect           testSummary
	}{
		"undefined": {
			testSummary: testSummary{
				mu:           sync.Mutex{},
				enabledColor: false,
				passed:       []string{},
				failed:       []string{},
				skipped:      []string{},
			},
			testFileRelPath:  "scenario/test.yaml",
			testResultString: "undefined",
			expect: testSummary{
				mu:           sync.Mutex{},
				enabledColor: false,
				passed:       []string{},
				failed:       []string{},
				skipped:      []string{},
			},
		},
		"passed": {
			testSummary: testSummary{
				mu:           sync.Mutex{},
				enabledColor: false,
				passed:       []string{},
				failed:       []string{},
				skipped:      []string{},
			},
			testFileRelPath:  "scenario/test.yaml",
			testResultString: "passed",
			expect: testSummary{
				mu:           sync.Mutex{},
				enabledColor: false,
				passed:       []string{"scenario/test.yaml"},
				failed:       []string{},
				skipped:      []string{},
			},
		},
		"failed": {
			testSummary: testSummary{
				mu:           sync.Mutex{},
				enabledColor: false,
				passed:       []string{},
				failed:       []string{},
				skipped:      []string{},
			},
			testFileRelPath:  "scenario/test.yaml",
			testResultString: "failed",
			expect: testSummary{
				mu:           sync.Mutex{},
				enabledColor: false,
				passed:       []string{},
				failed:       []string{"scenario/test.yaml"},
				skipped:      []string{},
			},
		},
		"skipped": {
			testSummary: testSummary{
				mu:           sync.Mutex{},
				enabledColor: false,
				passed:       []string{},
				failed:       []string{},
				skipped:      []string{},
			},
			testFileRelPath:  "scenario/test.yaml",
			testResultString: "skipped",
			expect: testSummary{
				mu:           sync.Mutex{},
				enabledColor: false,
				passed:       []string{},
				failed:       []string{},
				skipped:      []string{"scenario/test.yaml"},
			},
		},
	}

	for name, test := range tests {
		tt := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tt.testSummary.add(tt.testFileRelPath, tt.testResultString)

			if diff := cmp.Diff(tt.expect, tt.testSummary,
				cmp.AllowUnexported(Runner{}, schema.OrderedMap[string, schema.PluginConfig]{}, testSummary{}),
				cmpopts.IgnoreFields(testSummary{}, "mu"),
			); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}
