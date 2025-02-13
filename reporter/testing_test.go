package reporter

import (
	"bytes"
	"os"
	"testing"

	"github.com/scenarigo/scenarigo/internal/testutil"
	"github.com/sergi/go-diff/diffmatchpatch"
)

func TestFromT(t *testing.T) {
	t.Run("no flag", func(t *testing.T) {
		args := os.Args
		t.Cleanup(func() { os.Args = args })
		os.Args = []string{}
		r := FromT(t).(*reporter)
		if r.context.verbose {
			t.Error("verbose should be false")
		}
		if r.context.matcher != nil {
			t.Fatal("matcher should be nil")
		}
	})
	t.Run("verbose", func(t *testing.T) {
		args := os.Args
		t.Cleanup(func() { os.Args = args })
		os.Args = []string{"-test.v=true"}
		r := FromT(t).(*reporter)
		if !r.context.verbose {
			t.Error("verbose should be true")
		}
	})
	t.Run("matcher", func(t *testing.T) {
		args := os.Args
		t.Cleanup(func() { os.Args = args })
		os.Args = []string{`-test.run=FromT/matcher/bar/^ho`}
		var b bytes.Buffer
		r := FromT(t, WithWriter(&b), WithVerboseLog())
		rptr := r.(*reporter)
		if rptr.context.matcher == nil {
			t.Fatal("matcher should not be nil")
		}

		var (
			called1 bool
			called2 bool
			called3 bool
		)
		r.Run("foo", func(r Reporter) {
			r.Run("hoge", func(r Reporter) {
				r.Fatalf("%s should not be executed", r.Name())
			})
			r.Run("fuga", func(r Reporter) {
				r.Fatalf("%s should not be executed", r.Name())
			})
		})
		r.Run("bar", func(r Reporter) {
			r.Run("hoge", func(r Reporter) {
				r.Run("1", func(r Reporter) {
					r.Logf("%s called", r.Name())
					called1 = true
				})
				r.Run("2", func(r Reporter) {
					r.Logf("%s called", r.Name())
					called2 = true
				})
			})
			r.Run("fuga", func(r Reporter) {
				r.Fatalf("%s should not be executed", r.Name())
			})
		})
		r.Run("bar/hoge", func(r Reporter) {
			r.Run("3", func(r Reporter) {
				r.Logf("%s called", r.Name())
				called3 = true
			})
		})
		if !called1 {
			t.Error("TestFromT/bar/hoge/1 should be executed")
		}
		if !called2 {
			t.Error("TestFromT/bar/hoge/2 should be executed")
		}
		if !called3 {
			t.Error("TestFromT/bar/hoge/3 should be executed")
		}
		expect := `=== RUN   TestFromT/matcher/bar
=== RUN   TestFromT/matcher/bar/hoge
=== RUN   TestFromT/matcher/bar/hoge/1
=== RUN   TestFromT/matcher/bar/hoge/2
--- PASS: TestFromT/matcher/bar (0.00s)
    --- PASS: TestFromT/matcher/bar/hoge (0.00s)
        --- PASS: TestFromT/matcher/bar/hoge/1 (0.00s)
                TestFromT/matcher/bar/hoge/1 called
        --- PASS: TestFromT/matcher/bar/hoge/2 (0.00s)
                TestFromT/matcher/bar/hoge/2 called
=== RUN   TestFromT/matcher/bar/hoge
=== RUN   TestFromT/matcher/bar/hoge/3
--- PASS: TestFromT/matcher/bar/hoge (0.00s)
    --- PASS: TestFromT/matcher/bar/hoge/3 (0.00s)
            TestFromT/matcher/bar/hoge/3 called
`
		if got := testutil.ReplaceOutput(b.String()); got != expect {
			dmp := diffmatchpatch.New()
			diffs := dmp.DiffMain(expect, got, false)
			t.Errorf("stdout differs:\n%s", dmp.DiffPrettyText(diffs))
		}
	})
}

func Test_TestingLog(t *testing.T) {
	rptr := FromT(t)
	rptr.Log("log test")
	rptr.Run("subtest", func(rptr Reporter) {
		rptr.Logf("%s", "log test")
		rptr.Skip()
	})
}
