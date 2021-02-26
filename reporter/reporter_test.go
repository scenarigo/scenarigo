package reporter

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestReporter(t *testing.T) {
	var _ Reporter = new()
}

func TestReporter_Name(t *testing.T) {
	name := "testname"
	r := new()
	r.goTestName = name
	if expect, got := name, r.Name(); got != expect {
		t.Errorf(`expected "%s" but got "%s"`, expect, got)
	}
}

func TestReporter_Fail(t *testing.T) {
	r := new()
	r.Fail()
	if expect, got := true, r.Failed(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
}

func TestReporter_Failed(t *testing.T) {
	r := new()
	if expect, got := false, r.Failed(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
	r.failed = 1
	if expect, got := true, r.Failed(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
}

func TestReporter_FailNow(t *testing.T) {
	r := new()
	done := make(chan bool)
	var reached bool
	go func() {
		defer func() {
			close(done)
		}()
		r.FailNow()
		reached = true // should not be reached
	}()
	<-done
	if expect, got := true, r.Failed(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
	if expect, got := false, reached; got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
}

func TestReporter_Log(t *testing.T) {
	str := "log"
	r := new()
	r.Log(str)
	if expect, got := 1, len(r.logs.all()); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := str, r.logs.all()[0]; got != expect {
		t.Errorf(`expected "%s" but got "%s"`, expect, got)
	}
}

func TestReporter_Logf(t *testing.T) {
	format := "%s failed"
	name := "testname"
	r := new()
	r.Errorf(format, name)
	if expect, got := 1, len(r.logs.all()); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := fmt.Sprintf(format, name), r.logs.all()[0]; got != expect {
		t.Errorf(`expected "%s" but got "%s"`, expect, got)
	}
}

func TestReporter_Error(t *testing.T) {
	name := "testname"
	r := new()
	r.Error(name)
	if expect, got := true, r.Failed(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
	if expect, got := 1, len(r.logs.all()); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := name, r.logs.all()[0]; got != expect {
		t.Errorf(`expected "%s" but got "%s"`, expect, got)
	}
}

func TestReporter_Errorf(t *testing.T) {
	format := "%s failed"
	name := "testname"
	r := new()
	r.Errorf(format, name)
	if expect, got := true, r.Failed(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
	if expect, got := 1, len(r.logs.all()); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := fmt.Sprintf(format, name), r.logs.all()[0]; got != expect {
		t.Errorf(`expected "%s" but got "%s"`, expect, got)
	}
}

func TestReporter_Fatal(t *testing.T) {
	name := "testname"
	r := new()
	done := make(chan bool)
	var reached bool
	go func() {
		defer func() {
			close(done)
		}()
		r.Fatal(name)
		reached = true // should not be reached
	}()
	<-done
	if expect, got := true, r.Failed(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
	if expect, got := 1, len(r.logs.all()); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := name, r.logs.all()[0]; got != expect {
		t.Errorf(`expected "%s" but got "%s"`, expect, got)
	}
	if expect, got := false, reached; got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
}

func TestReporter_Fatalf(t *testing.T) {
	format := "%s failed"
	name := "testname"
	r := new()
	done := make(chan bool)
	var reached bool
	go func() {
		defer func() {
			close(done)
		}()
		r.Fatalf(format, name)
		reached = true // should not be reached
	}()
	<-done
	if expect, got := true, r.Failed(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
	if expect, got := 1, len(r.logs.all()); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := fmt.Sprintf(format, name), r.logs.all()[0]; got != expect {
		t.Errorf(`expected "%s" but got "%s"`, expect, got)
	}
	if expect, got := false, reached; got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
}

func TestReporter_Skip(t *testing.T) {
	name := "testname"
	r := new()
	done := make(chan bool)
	var reached bool
	go func() {
		defer func() {
			close(done)
		}()
		r.Skip(name)
		reached = true // should not be reached
	}()
	<-done
	if expect, got := true, r.Skipped(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
	if expect, got := 1, len(r.logs.all()); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := name, r.logs.all()[0]; got != expect {
		t.Errorf(`expected "%s" but got "%s"`, expect, got)
	}
	if expect, got := false, reached; got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
}

func TestReporter_Skipf(t *testing.T) {
	format := "%s skipped"
	name := "testname"
	r := new()
	done := make(chan bool)
	var reached bool
	go func() {
		defer func() {
			close(done)
		}()
		r.Skipf(format, name)
		reached = true // should not be reached
	}()
	<-done
	if expect, got := true, r.Skipped(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
	if expect, got := 1, len(r.logs.all()); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := fmt.Sprintf(format, name), r.logs.all()[0]; got != expect {
		t.Errorf(`expected "%s" but got "%s"`, expect, got)
	}
	if expect, got := false, reached; got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
}

func TestReporter_SkipNow(t *testing.T) {
	r := new()
	done := make(chan bool)
	var reached bool
	go func() {
		defer func() {
			close(done)
		}()
		r.SkipNow()
		reached = true // should not be reached
	}()
	<-done
	if expect, got := true, r.Skipped(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
	if expect, got := false, reached; got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
}

func TestReporter_Skipped(t *testing.T) {
	r := new()
	if expect, got := false, r.Skipped(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
	r.skipped = 1
	if expect, got := true, r.Skipped(); got != expect {
		t.Errorf("expected %t but got %t", expect, got)
	}
}

type result struct {
	Failed   bool
	Skipped  bool
	Logs     []string
	Children []result
}

func TestPrint(t *testing.T) {
	pr := func(t *testing.T, r Reporter) *reporter {
		t.Helper()
		rptr, ok := r.(*reporter)
		if !ok {
			t.Fatalf("expected *reporter but got %T", r)
		}
		return rptr
	}

	tests := map[string]struct {
		f      func(*testing.T, *reporter)
		expect string
	}{
		"ok": {
			f: func(t *testing.T, r *reporter) {
				r.Run("a", func(r Reporter) {
					rptr := pr(t, r)
					rptr.durationMeasurer = &fixedDurationMeasurer{
						duration: 1234 * time.Millisecond,
					}
				})
			},
			expect: `
ok  	a	1.234s
`,
		},
		"FAIL": {
			f: func(t *testing.T, r *reporter) {
				r.Run("a", func(r Reporter) {
					rptr := pr(t, r)
					rptr.Error("error!")
					rptr.durationMeasurer = &fixedDurationMeasurer{
						duration: 1234 * time.Millisecond,
					}
				})
			},
			expect: `
--- FAIL: a (1.23s)
        error!
FAIL
FAIL	a	1.234s
FAIL
`,
		},
		"ok nest": {
			f: func(t *testing.T, r *reporter) {
				r.Run("a", func(r Reporter) {
					r.Run("b", func(r Reporter) {
						r.Run("c", func(r Reporter) {
							r.Log("ok!")
						})
					})
				})
			},
			expect: `
ok  	a	0.000s
`,
		},
		"FAIL nest": {
			f: func(t *testing.T, r *reporter) {
				r.Run("a", func(r Reporter) {
					r.Run("b", func(r Reporter) {
						r.Run("c", func(r Reporter) {
							rptr := pr(t, r)
							rptr.Error("error!")
							rptr.durationMeasurer = &fixedDurationMeasurer{
								duration: 1230 * time.Millisecond,
							}
						})
					})
				})
			},
			expect: `
--- FAIL: a (0.00s)
    --- FAIL: a/b (0.00s)
        --- FAIL: a/b/c (1.23s)
                error!
FAIL
FAIL	a	0.000s
FAIL
`,
		},
		"ok nest verbose": {
			f: func(t *testing.T, r *reporter) {
				r.context.verbose = true
				r.Run("a", func(r Reporter) {
					r.Run("b", func(r Reporter) {
						r.Run("c", func(r Reporter) {
							r.Log("ok!")
						})
					})
				})
			},
			expect: `
=== RUN   a
=== RUN   a/b
=== RUN   a/b/c
--- PASS: a (0.00s)
    --- PASS: a/b (0.00s)
        --- PASS: a/b/c (0.00s)
                ok!
PASS
ok  	a	0.000s
`,
		},
		"FAIL nest verbose": {
			f: func(t *testing.T, r *reporter) {
				r.context.verbose = true
				r.Run("a", func(r Reporter) {
					r.Run("b", func(r Reporter) {
						r.Run("c", func(r Reporter) {
							rptr := pr(t, r)
							rptr.Error("error!")
							rptr.durationMeasurer = &fixedDurationMeasurer{
								duration: 1230 * time.Millisecond,
							}
						})
					})
				})
			},
			expect: `
=== RUN   a
=== RUN   a/b
=== RUN   a/b/c
--- FAIL: a (0.00s)
    --- FAIL: a/b (0.00s)
        --- FAIL: a/b/c (1.23s)
                error!
FAIL
FAIL	a	0.000s
FAIL
`,
		},
		"multi line log": {
			f: func(t *testing.T, r *reporter) {
				r.Run("a", func(r Reporter) {
					r.Run("b", func(r Reporter) {
						r.Run("c", func(r Reporter) {
							rptr := pr(t, r)
							rptr.Log("1\n2\n3")
							rptr.FailNow()
						})
					})
				})
			},
			expect: `
--- FAIL: a (0.00s)
    --- FAIL: a/b (0.00s)
        --- FAIL: a/b/c (0.00s)
                1
                2
                3
FAIL
FAIL	a	0.000s
FAIL
`,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			Run(func(r Reporter) {
				rptr := pr(t, r)
				rptr.durationMeasurer = &fixedDurationMeasurer{}
				test.f(t, rptr)
			}, WithWriter(&b))
			if diff := cmp.Diff(test.expect, "\n"+b.String()); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReporter_PrivateMethods(t *testing.T) {
	tests := map[string]struct {
		run      func(t *testing.T, f func(Reporter))
		rootName string
	}{
		"reporter": {
			run: func(t *testing.T, f func(Reporter)) {
				Run(f)
			},
		},
		"testReporter": {
			run: func(t *testing.T, f func(Reporter)) {
				f(FromT(t))
			},
			rootName: "TestReporter_PrivateMethods/testReporter",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			var r Reporter
			test.run(t, func(rptr Reporter) {
				r = rptr
				if ok := r.Run("child", func(r Reporter) {
					time.Sleep(10 * time.Millisecond)
					r.Log("child log")
				}); !ok {
					t.Fatal("test failed")
				}
				if ok := r.Run("skip", func(r Reporter) {
					time.Sleep(10 * time.Millisecond)
					r.Skip("skip log")
				}); !ok {
					t.Fatal("test failed")
				}
			})

			children := r.getChildren()
			if got := len(children); got != 2 {
				t.Fatalf("expected length is 2 but %d", got)
			}
			t.Run("getName", func(t *testing.T) {
				if got, expected := r.getName(), test.rootName; got != expected {
					t.Errorf("expect %q but got %q", expected, got)
				}
				if got, expected := children[0].getName(), "child"; got != expected {
					t.Errorf("expect %q but got %q", expected, got)
				}
				if got, expected := children[1].getName(), "skip"; got != expected {
					t.Errorf("expect %q but got %q", expected, got)
				}
			})
			t.Run("getDuration", func(t *testing.T) {
				if got := r.getDuration(); got == 0 {
					t.Error("duration must be greater than 0")
				}
				if got := children[0].getDuration(); got == 0 {
					t.Error("duration must be greater than 0")
				}
				if got := children[1].getDuration(); got == 0 {
					t.Error("duration must be greater than 0")
				}
			})
			t.Run("getLogs", func(t *testing.T) {
				opts := []cmp.Option{
					cmp.AllowUnexported(logRecorder{}),
					cmp.FilterPath(func(p cmp.Path) bool {
						return p.Last().String() == ".m"
					}, cmp.Ignore()),
				}
				if diff := cmp.Diff(&logRecorder{}, r.getLogs(), opts...); diff != "" {
					t.Errorf("result mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(&logRecorder{
					strs:     []string{"child log"},
					infoIdxs: []int{0},
				}, children[0].getLogs(), opts...); diff != "" {
					t.Errorf("result mismatch (-want +got):\n%s", diff)
				}
				zero := 0
				if diff := cmp.Diff(&logRecorder{
					strs:    []string{"skip log"},
					skipIdx: &zero,
				}, children[1].getLogs(), opts...); diff != "" {
					t.Errorf("result mismatch (-want +got):\n%s", diff)
				}
			})
			t.Run("isRoot", func(t *testing.T) {
				if got, expected := r.isRoot(), true; got != expected {
					t.Errorf("expect %t but got %t", expected, got)
				}
				if got, expected := children[0].isRoot(), false; got != expected {
					t.Errorf("expect %t but got %t", expected, got)
				}
				if got, expected := children[1].isRoot(), false; got != expected {
					t.Errorf("expect %t but got %t", expected, got)
				}
			})
		})
	}
}
