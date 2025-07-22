package context

import (
	"testing"

	"github.com/scenarigo/scenarigo/reporter"
)

func TestSteps(t *testing.T) {
	steps := NewSteps()
	steps.Add("foo", &Step{
		Result: reporter.TestResultPassed.String(),
	})
	steps.Add("bar", &Step{
		Result: reporter.TestResultFailed.String(),
	})
	t.Run("get foo", func(t *testing.T) {
		step := steps.Get("foo")
		if step == nil {
			t.Fatal("failed to get")
			return
		}
		if got, expect := step.Result, "passed"; got != expect {
			t.Errorf("expect %q but got %q", expect, got)
		}
	})
	t.Run("get bar", func(t *testing.T) {
		step := steps.Get("bar")
		if step == nil {
			t.Fatal("failed to get")
			return
		}
		if got, expect := step.Result, "failed"; got != expect {
			t.Errorf("expect %q but got %q", expect, got)
		}
	})
	t.Run("get baz", func(t *testing.T) {
		step := steps.Get("baz")
		if step != nil {
			t.Fatal("should be nil")
		}
	})
	t.Run("ExtractByKey", func(t *testing.T) {
		if v, ok := steps.ExtractByKey("foo"); !ok {
			t.Fatal("not found")
		} else {
			step, ok := v.(*Step)
			if !ok {
				t.Fatalf("expect *Step but got %T", v)
			}
			if got, expect := step.Result, "passed"; got != expect {
				t.Errorf("expect %q but got %q", expect, got)
			}
		}
		if _, ok := steps.ExtractByKey("baz"); ok {
			t.Fatal("found")
		}
	})
}
