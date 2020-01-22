package reporter

import (
	"bytes"
	"fmt"
	"runtime"
	"strconv"
	"strings"
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
	r.name = name
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
	if expect, got := 1, len(r.logs); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := str, r.logs[0]; got != expect {
		t.Errorf(`expected "%s" but got "%s"`, expect, got)
	}
}

func TestReporter_Logf(t *testing.T) {
	format := "%s failed"
	name := "testname"
	r := new()
	r.Errorf(format, name)
	if expect, got := 1, len(r.logs); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := fmt.Sprintf(format, name), r.logs[0]; got != expect {
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
	if expect, got := 1, len(r.logs); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := name, r.logs[0]; got != expect {
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
	if expect, got := 1, len(r.logs); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := fmt.Sprintf(format, name), r.logs[0]; got != expect {
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
	if expect, got := 1, len(r.logs); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := name, r.logs[0]; got != expect {
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
	if expect, got := 1, len(r.logs); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := fmt.Sprintf(format, name), r.logs[0]; got != expect {
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
	if expect, got := 1, len(r.logs); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := name, r.logs[0]; got != expect {
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
	if expect, got := 1, len(r.logs); got != expect {
		t.Fatalf("expected length %d but got %d", expect, got)
	}
	if expect, got := fmt.Sprintf(format, name), r.logs[0]; got != expect {
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

func collectResult(r *reporter) result {
	res := result{
		Failed:  r.Failed(),
		Skipped: r.Skipped(),
		Logs:    r.logs,
	}
	for _, child := range r.children {
		res.Children = append(res.Children, collectResult(child))
	}
	return res
}

func ignoreStackTrace(in result) result {
	out := result{
		Failed:  in.Failed,
		Skipped: in.Skipped,
	}
	for _, l := range in.Logs {
		if !strings.HasPrefix(l, "goroutine ") {
			out.Logs = append(out.Logs, l)
		}
	}
	for _, child := range in.Children {
		out.Children = append(out.Children, ignoreStackTrace(child))
	}
	return out
}

func TestRun(t *testing.T) {
	tests := map[string]struct {
		maxParallel            int
		fs                     []func(r Reporter)
		expect                 result
		expectSerialDuration   time.Duration
		expectParallelDuration time.Duration
	}{
		"all tests passed": {
			fs: []func(r Reporter){
				func(r Reporter) {},
				func(r Reporter) {},
			},
			expect: result{
				Children: []result{
					{},
					{},
				},
			},
		},
		"child failed": {
			fs: []func(r Reporter){
				func(r Reporter) {},
				func(r Reporter) {
					r.Fail()
				},
			},
			expect: result{
				Failed: true,
				Children: []result{
					{},
					{
						Failed: true,
					},
				},
			},
		},
		"child skipped": {
			fs: []func(r Reporter){
				func(r Reporter) {},
				func(r Reporter) {
					r.SkipNow()
				},
			},
			expect: result{
				Children: []result{
					{},
					{
						Skipped: true,
					},
				},
			},
		},
		`child panic("panic!")`: {
			fs: []func(r Reporter){
				func(r Reporter) {},
				func(r Reporter) {
					panic("panic!")
				},
			},
			expect: result{
				Failed: true,
				Children: []result{
					{},
					{
						Failed: true,
						Logs:   []string{"panic!"},
					},
				},
			},
		},
		"child panic(nil)": {
			fs: []func(r Reporter){
				func(r Reporter) {},
				func(r Reporter) {
					panic(nil)
				},
			},
			expect: result{
				Failed: true,
				Children: []result{
					{},
					{
						Failed: true,
						Logs:   []string{"test executed panic(nil) or runtime.Goexit"},
					},
				},
			},
		},
		"child runtime.Goexit()": {
			fs: []func(r Reporter){
				func(r Reporter) {},
				func(r Reporter) {
					runtime.Goexit()
				},
			},
			expect: result{
				Failed: true,
				Children: []result{
					{},
					{
						Failed: true,
						Logs:   []string{"test executed panic(nil) or runtime.Goexit"},
					},
				},
			},
		},
		"run all tests in parallel": {
			maxParallel: 3,
			fs: []func(r Reporter){
				func(r Reporter) {
					time.Sleep(100 * time.Millisecond)
				},
				func(r Reporter) {
					time.Sleep(100 * time.Millisecond)
				},
				func(r Reporter) {
					time.Sleep(100 * time.Millisecond)
				},
			},
			expect: result{
				Children: []result{
					{},
					{},
					{},
				},
			},
			expectSerialDuration:   300 * time.Millisecond,
			expectParallelDuration: 100 * time.Millisecond,
		},
		"run tests in parallel (max number of concurrent executions is 2)": {
			maxParallel: 2,
			fs: []func(r Reporter){
				func(r Reporter) {
					time.Sleep(100 * time.Millisecond)
				},
				func(r Reporter) {
					time.Sleep(100 * time.Millisecond)
				},
				func(r Reporter) {
					time.Sleep(100 * time.Millisecond)
				},
			},
			expect: result{
				Children: []result{
					{},
					{},
					{},
				},
			},
			expectSerialDuration:   300 * time.Millisecond,
			expectParallelDuration: 200 * time.Millisecond,
		},
	}
	for name, test := range tests {
		test := test
		opts := []Option{}
		if test.maxParallel != 0 {
			opts = append(opts, WithMaxParallel(test.maxParallel))
		}
		t.Run(name, func(t *testing.T) {
			t.Run("serial", func(t *testing.T) {
				start := time.Now()
				r := run(func(r Reporter) {
					for i, f := range test.fs {
						f := f
						r.Run(strconv.Itoa(i), func(r Reporter) {
							f(r)
						})
					}
				}, opts...)
				duration := time.Since(start)
				if diff := cmp.Diff(test.expect, ignoreStackTrace(collectResult(r))); diff != "" {
					t.Errorf("result mismatch (-want +got):\n%s", diff)
				}
				if expect, got := test.expectSerialDuration, duration.Truncate(50*time.Millisecond); got != expect {
					t.Errorf("expected %s but got %s", expect, got)
				}
			})
			t.Run("parallel", func(t *testing.T) {
				start := time.Now()
				r := run(func(r Reporter) {
					for i, f := range test.fs {
						f := f
						r.Run(strconv.Itoa(i), func(r Reporter) {
							r.Parallel()
							f(r)
						})
					}
				}, opts...)
				duration := time.Since(start)
				if diff := cmp.Diff(test.expect, ignoreStackTrace(collectResult(r))); diff != "" {
					t.Errorf("result mismatch (-want +got):\n%s", diff)
				}
				if expect, got := test.expectParallelDuration, duration.Truncate(50*time.Millisecond); got != expect {
					t.Errorf("expected %s but got %s", expect, got)
				}
			})
		})
	}
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
					rptr.duration = 1234 * time.Millisecond
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
					rptr.duration = 1234 * time.Millisecond
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
							rptr.duration = 1230 * time.Millisecond
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
							rptr.duration = 1230 * time.Millisecond
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
				test.f(t, rptr)
			}, WithWriter(&b))
			if diff := cmp.Diff(test.expect, "\n"+b.String()); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
