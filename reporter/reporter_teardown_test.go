package reporter

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"

	"github.com/scenarigo/scenarigo/internal/testutil"
)

func TestReporter_Teardown(t *testing.T) {
	pr := func(t *testing.T, r Reporter) *reporter {
		t.Helper()
		rptr, ok := r.(*reporter)
		if !ok {
			t.Fatalf("expected *reporter but got %T", r)
		}
		return rptr
	}

	tests := map[string]struct {
		f             func(*testing.T, *reporter)
		parallelTests []string
		expect        string
		checkPanic    bool
	}{
		"teardown is called after all parallel tests complete": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				// Register teardown functions
				r.Teardown("teardown1", func(r Reporter) {
					r.Log("teardown1")
				})
				r.Teardown("teardown2", func(r Reporter) {
					r.Log("teardown2")
				})

				// Run parallel tests
				r.Run("test1", func(r Reporter) {
					r.Parallel()
					r.Log("test1 start")
					time.Sleep(10 * time.Millisecond) // Reduced for test speed
					r.Log("test1 end")
				})
				r.Run("test2", func(r Reporter) {
					r.Parallel()
					r.Log("test2 start")
					time.Sleep(5 * time.Millisecond) // Reduced for test speed
					r.Log("test2 end")
				})
				r.Run("test3", func(r Reporter) {
					r.Log("test3 sequential")
				})
			},
			parallelTests: []string{"test1", "test2"},
			expect: `
=== RUN   test1
=== PAUSE test1
=== RUN   test2
=== PAUSE test2
=== RUN   test3
--- PASS: test3 (0.00s)
        test3 sequential
PASS
ok  	test3	0.000s
=== CONT  test1
=== CONT  test2
--- PASS: test2 (0.00s)
        test2 start
        test2 end
PASS
ok  	test2	0.000s
--- PASS: test1 (0.00s)
        test1 start
        test1 end
PASS
ok  	test1	0.000s
=== RUN   teardown2
--- PASS: teardown2 (0.00s)
        teardown2
PASS
ok  	teardown2	0.000s
=== RUN   teardown1
--- PASS: teardown1 (0.00s)
        teardown1
PASS
ok  	teardown1	0.000s
`,
		},

		"teardown handles panics gracefully": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				r.Teardown("first_teardown", func(r Reporter) {
					panic("first teardown panic")
				})

				r.Teardown("second_teardown", func(r Reporter) {
					r.Log("second teardown called")
				})

				r.Teardown("third_teardown", func(r Reporter) {
					panic("third teardown panic")
				})

				r.Run("test", func(r Reporter) {
					r.Log("test completed")
				})
			},
			expect: `ok  	test	0.000s
third teardown panic
second teardown called
teardown panic`,
			checkPanic: true,
		},

		"teardown with no parallel tests": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				r.Teardown("teardown", func(r Reporter) {
					r.Log("teardown called")
				})

				r.Run("test1", func(r Reporter) {
					r.Log("test1 sequential")
				})

				r.Run("test2", func(r Reporter) {
					r.Log("test2 sequential")
				})
			},
			expect: `
=== RUN   test1
--- PASS: test1 (0.00s)
        test1 sequential
PASS
ok  	test1	0.000s
=== RUN   test2
--- PASS: test2 (0.00s)
        test2 sequential
PASS
ok  	test2	0.000s
=== RUN   teardown
--- PASS: teardown (0.00s)
        teardown called
PASS
ok  	teardown	0.000s
`,
		},

		"nested teardowns": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				r.Teardown("parent_teardown", func(r Reporter) {
					r.Log("parent teardown")
				})

				r.Run("subtest", func(r Reporter) {
					r.Teardown("child_teardown", func(r Reporter) {
						r.Log("child teardown")
					})

					r.Run("nested", func(r Reporter) {
						r.Parallel()
						r.Log("nested test")
						time.Sleep(5 * time.Millisecond) // Reduced for test speed
					})
				})
			},
			parallelTests: []string{"subtest/nested"},
			expect: `
=== RUN   subtest
=== RUN   subtest/nested
=== PAUSE subtest/nested
=== CONT  subtest/nested
=== RUN   subtest/child_teardown
--- PASS: subtest (0.00s)
    --- PASS: subtest/nested (0.00s)
            nested test
    --- PASS: subtest/child_teardown (0.00s)
            child teardown
PASS
ok  	subtest	0.000s
=== RUN   parent_teardown
--- PASS: parent_teardown (0.00s)
        parent teardown
PASS
ok  	parent_teardown	0.000s
`,
		},

		"teardown with parallel run": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				r.Teardown("parallel_teardown", func(r Reporter) {
					r.Log("teardown start")
					// This should now work properly as a subtest
					r.Run("teardown_parallel", func(r Reporter) {
						r.Parallel()
						r.Log("parallel in teardown")
						time.Sleep(5 * time.Millisecond)
					})
					r.Log("teardown end")
				})

				r.Run("main_test", func(r Reporter) {
					r.Log("main test")
				})
			},
			parallelTests: []string{"parallel_teardown/teardown_parallel"},
			expect: `
=== RUN   main_test
--- PASS: main_test (0.00s)
        main test
PASS
ok  	main_test	0.000s
=== RUN   parallel_teardown
=== RUN   parallel_teardown/teardown_parallel
=== PAUSE parallel_teardown/teardown_parallel
=== CONT  parallel_teardown/teardown_parallel
--- PASS: parallel_teardown (0.00s)
        teardown start
        teardown end
    --- PASS: parallel_teardown/teardown_parallel (0.00s)
            parallel in teardown
PASS
ok  	parallel_teardown	0.000s
`,
		},

		"teardown order with main parallel tests": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				// Register multiple teardowns
				r.Teardown("teardown1", func(r Reporter) {
					r.Log("teardown1 executed")
				})

				r.Teardown("teardown2", func(r Reporter) {
					r.Log("teardown2 executed")
					// This teardown has a parallel subtest
					r.Run("teardown2_parallel", func(r Reporter) {
						r.Parallel()
						r.Log("teardown2 parallel work")
						time.Sleep(5 * time.Millisecond)
					})
					r.Log("teardown2 finished")
				})

				// Run main parallel tests
				r.Run("main1", func(r Reporter) {
					r.Parallel()
					r.Log("main1 parallel")
					time.Sleep(10 * time.Millisecond)
				})

				r.Run("main2", func(r Reporter) {
					r.Parallel()
					r.Log("main2 parallel")
					time.Sleep(5 * time.Millisecond)
				})
			},
			parallelTests: []string{"main1", "main2"},
			expect: `
=== RUN   main1
=== PAUSE main1
=== RUN   main2
=== PAUSE main2
=== CONT  main1
=== CONT  main2
--- PASS: main2 (0.00s)
        main2 parallel
PASS
ok  	main2	0.000s
--- PASS: main1 (0.00s)
        main1 parallel
PASS
ok  	main1	0.000s
=== RUN   teardown2
=== RUN   teardown2/teardown2_parallel
=== PAUSE teardown2/teardown2_parallel
=== CONT  teardown2/teardown2_parallel
--- PASS: teardown2 (0.00s)
        teardown2 executed
        teardown2 finished
    --- PASS: teardown2/teardown2_parallel (0.00s)
            teardown2 parallel work
PASS
ok  	teardown2	0.000s
=== RUN   teardown1
--- PASS: teardown1 (0.00s)
        teardown1 executed
PASS
ok  	teardown1	0.000s
`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			Run(func(r Reporter) {
				rptr := pr(t, r)
				rptr.durationMeasurer = &fixedDurationMeasurer{}
				test.f(t, rptr)
			}, WithWriter(&b), WithVerboseLog(), WithMaxParallel(10))

			actual := "\n" + b.String()
			if test.checkPanic {
				// For panic tests, check that the output contains expected lines
				// but ignore the stack trace part
				lines := strings.Split(actual, "\n")
				expectedLines := strings.Split(test.expect, "\n")

				// Check that each expected line exists in actual output
				for _, expectedLine := range expectedLines {
					if expectedLine == "" {
						continue
					}
					found := false
					for _, actualLine := range lines {
						if strings.Contains(actualLine, expectedLine) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected line %q not found in output", expectedLine)
					}
				}
			} else {
				normalizedActual := testutil.NormalizeParallelOutput(actual, test.parallelTests)
				normalizedExpected := testutil.NormalizeParallelOutput(test.expect, test.parallelTests)
				if normalizedActual != normalizedExpected {
					dmp := diffmatchpatch.New()
					diffs := dmp.DiffMain(normalizedExpected, normalizedActual, false)
					t.Errorf("result mismatch:\n%s", dmp.DiffPrettyText(diffs))
				}
			}
		})
	}
}

func TestReporter_Cleanup(t *testing.T) {
	pr := func(t *testing.T, r Reporter) *reporter {
		t.Helper()
		rptr, ok := r.(*reporter)
		if !ok {
			t.Fatalf("expected *reporter but got %T", r)
		}
		return rptr
	}

	tests := map[string]struct {
		f          func(*testing.T, *reporter)
		expect     string
		checkPanic bool
	}{
		"cleanup is called after test completion": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				// Register cleanup functions
				r.Cleanup(func() {
					r.Log("cleanup1")
				})
				r.Cleanup(func() {
					r.Log("cleanup2")
				})

				// Run a simple test
				r.Run("test1", func(r Reporter) {
					r.Log("test1")
				})
			},
			expect: `=== RUN   test1
--- PASS: test1 (0.00s)
        test1
PASS
ok  	test1	0.000s
cleanup2
cleanup1
`,
		},

		"cleanup order is LIFO": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				// Register multiple cleanup functions
				r.Cleanup(func() {
					r.Log("cleanup first")
				})
				r.Cleanup(func() {
					r.Log("cleanup second")
				})
				r.Cleanup(func() {
					r.Log("cleanup third")
				})
			},
			expect: `cleanup third
cleanup second
cleanup first
`,
		},

		"cleanup runs after teardown": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				// Register both cleanup and teardown
				r.Cleanup(func() {
					r.Log("cleanup executed")
				})
				r.Teardown("teardown", func(r Reporter) {
					r.Log("teardown executed")
				})
			},
			expect: `=== RUN   teardown
--- PASS: teardown (0.00s)
        teardown executed
PASS
ok  	teardown	0.000s
cleanup executed
`,
		},

		"cleanup handles panics gracefully": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				// Register cleanup that panics
				r.Cleanup(func() {
					panic("cleanup panic")
				})
				r.Cleanup(func() {
					r.Log("cleanup after panic")
				})
			},
			checkPanic: true, // We expect panic to be caught and reported
		},

		"run called during cleanup causes panic": {
			f: func(t *testing.T, r *reporter) {
				t.Helper()

				// Register cleanup that tries to call Run
				r.Cleanup(func() {
					r.Log("cleanup before run")
					r.Run("invalid", func(r Reporter) {
						r.Log("this should not execute")
					})
					r.Log("cleanup after run")
				})
			},
			checkPanic: true, // We expect Run to panic when called during cleanup
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			opts := []Option{
				WithWriter(&out),
				WithVerboseLog(),
				WithMaxParallel(10),
			}

			Run(func(r Reporter) {
				rptr := pr(t, r)
				rptr.durationMeasurer = &fixedDurationMeasurer{}
				test.f(t, rptr)
			}, opts...)

			actual := out.String()

			if test.checkPanic {
				// For panic tests, just check that the panic was caught and reported
				if !strings.Contains(actual, "panic") {
					t.Errorf("expected panic to be caught and reported, but got: %s", actual)
				}
			} else {
				normalizedActual := testutil.NormalizeParallelOutput(actual, []string{})
				normalizedExpected := testutil.NormalizeParallelOutput(test.expect, []string{})
				if normalizedActual != normalizedExpected {
					dmp := diffmatchpatch.New()
					diffs := dmp.DiffMain(normalizedExpected, normalizedActual, false)
					t.Errorf("result mismatch:\n%s", dmp.DiffPrettyText(diffs))
				}
			}
		})
	}
}
