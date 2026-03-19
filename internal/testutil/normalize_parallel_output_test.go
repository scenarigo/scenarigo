package testutil

import (
	"testing"

	"github.com/sergi/go-diff/diffmatchpatch"
)

func TestNormalizeParallelOutput(t *testing.T) {
	tests := map[string]struct {
		input     string
		testnames []string
		expected  string
	}{
		"sorts parallel tests": {
			input: `
ok  	test3	0.000s
ok  	test2	0.000s
ok  	test1	0.000s`,
			testnames: []string{"test1", "test2", "test3"},
			expected: `
ok  	test1	0.000s
ok  	test2	0.000s
ok  	test3	0.000s`,
		},
		"sorts parallel tests (verbose)": {
			input: `
=== RUN   test1
=== PAUSE test1
=== RUN   test2
=== PAUSE test2
=== RUN   test3
=== PAUSE test3
=== CONT  test2
=== CONT  test1
=== CONT  test3
--- PASS: test3 (0.00s)
        test3 start
        test3 end
PASS
ok  	test3	0.000s
--- PASS: test2 (0.00s)
        test2 start
        test2 end
PASS
ok  	test2	0.000s
--- PASS: test1 (0.00s)
        test1 start
        test1 end
PASS
ok  	test1	0.000s`,
			testnames: []string{"test1", "test2", "test3"},
			expected: `
=== RUN   test1
=== PAUSE test1
=== RUN   test2
=== PAUSE test2
=== RUN   test3
=== PAUSE test3
=== CONT  test1
=== CONT  test2
=== CONT  test3
--- PASS: test1 (0.00s)
        test1 start
        test1 end
PASS
ok  	test1	0.000s
--- PASS: test2 (0.00s)
        test2 start
        test2 end
PASS
ok  	test2	0.000s
--- PASS: test3 (0.00s)
        test3 start
        test3 end
PASS
ok  	test3	0.000s`,
		},
		"sorts parallel test fail blocks": {
			input: `
--- FAIL: test2 (0.00s)
        test2 failed
FAIL
FAIL 	test2	0.000s
FAIL
ok  	test3	0.000s
--- FAIL: test1 (0.00s)
        test1 failed
FAIL
FAIL 	test1	0.000s
FAIL`,
			testnames: []string{"test1", "test2", "test3"},
			expected: `
--- FAIL: test1 (0.00s)
        test1 failed
FAIL
FAIL 	test1	0.000s
FAIL
--- FAIL: test2 (0.00s)
        test2 failed
FAIL
FAIL 	test2	0.000s
FAIL
ok  	test3	0.000s`,
		},
		"sorts parallel test fail blocks (verbose)": {
			input: `
=== RUN   test1
=== PAUSE test1
=== RUN   test2
=== PAUSE test2
=== RUN   test3
=== PAUSE test3
=== CONT  test3
=== CONT  test1
=== CONT  test2
--- FAIL: test2 (0.00s)
        test2 failed
FAIL
FAIL 	test2	0.000s
FAIL
--- PASS: test3 (0.00s)
        test3 start
        test3 end
PASS
ok  	test3	0.000s
--- FAIL: test1 (0.00s)
        test1 failed
FAIL
FAIL 	test1	0.000s
FAIL`,
			testnames: []string{"test1", "test2", "test3"},
			expected: `
=== RUN   test1
=== PAUSE test1
=== RUN   test2
=== PAUSE test2
=== RUN   test3
=== PAUSE test3
=== CONT  test1
=== CONT  test2
=== CONT  test3
--- FAIL: test1 (0.00s)
        test1 failed
FAIL
FAIL 	test1	0.000s
FAIL
--- FAIL: test2 (0.00s)
        test2 failed
FAIL
FAIL 	test2	0.000s
FAIL
--- PASS: test3 (0.00s)
        test3 start
        test3 end
PASS
ok  	test3	0.000s`,
		},
		"leaves non-parallel tests unchanged": {
			input: `
=== RUN   test2
--- PASS: test2 (0.00s)
        test2 log
PASS
ok  	test2	0.000s
=== RUN   test1
--- PASS: test1 (0.00s)
        test1 log
PASS
ok  	test1	0.000s`,
			testnames: []string{},
			expected: `
=== RUN   test2
--- PASS: test2 (0.00s)
        test2 log
PASS
ok  	test2	0.000s
=== RUN   test1
--- PASS: test1 (0.00s)
        test1 log
PASS
ok  	test1	0.000s`,
		},
		"handles interleaved parallel and sequential tests": {
			input: `
=== RUN   sequential1
--- PASS: sequential1 (0.00s)
PASS
ok  	sequential1	0.000s
=== RUN   parallel1
=== PAUSE parallel1
=== RUN   parallel2
=== PAUSE parallel2
=== CONT  parallel2
=== CONT  parallel1
--- PASS: parallel2 (0.00s)
        parallel2 log
PASS
ok  	parallel2	0.000s
=== RUN   sequential2
--- PASS: sequential2 (0.00s)
PASS
ok  	sequential2	0.000s
--- PASS: parallel1 (0.00s)
        parallel1 log
PASS
ok  	parallel1	0.000s`,
			testnames: []string{"parallel1", "parallel2"},
			expected: `
=== RUN   sequential1
--- PASS: sequential1 (0.00s)
PASS
ok  	sequential1	0.000s
=== RUN   parallel1
=== PAUSE parallel1
=== RUN   parallel2
=== PAUSE parallel2
=== CONT  parallel1
=== CONT  parallel2
--- PASS: parallel1 (0.00s)
        parallel1 log
PASS
ok  	parallel1	0.000s
--- PASS: parallel2 (0.00s)
        parallel2 log
PASS
ok  	parallel2	0.000s
=== RUN   sequential2
--- PASS: sequential2 (0.00s)
PASS
ok  	sequential2	0.000s`,
		},
		"handles empty input": {
			input:     "",
			testnames: []string{"test1"},
			expected:  "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := NormalizeParallelOutput(test.input, test.testnames)
			if actual != test.expected {
				dmp := diffmatchpatch.New()
				diffs := dmp.DiffMain(test.expected, actual, false)
				t.Errorf("NormalizeParallelOutput mismatch:\n%s", dmp.DiffPrettyText(diffs))
			}
		})
	}
}
