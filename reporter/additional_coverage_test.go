package reporter

import (
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/color"
)

func TestWithColorConfig(t *testing.T) {
	// Test WithColorConfig function
	colorConfig := color.New()
	colorConfig.SetEnabled(false)

	Run(func(r Reporter) {
		rep, ok := r.(*reporter)
		if !ok {
			t.Fatal("reporter is not of expected type")
		}
		if rep.context.colorConfig == nil {
			t.Error("WithColorConfig should set colorConfig")
		}
		if rep.context.colorConfig.IsEnabled() {
			t.Error("colorConfig should be disabled")
		}
	}, WithColorConfig(colorConfig))
}

func TestWithTestSummary(t *testing.T) {
	// Test WithTestSummary function
	var contextTestSummary bool
	var testSummaryObject interface{}

	Run(func(r Reporter) {
		rep, ok := r.(*reporter)
		if !ok {
			t.Fatal("reporter is not of expected type")
		}
		contextTestSummary = rep.context.enabledTestSummary
		testSummaryObject = rep.context.testSummary
	}, WithTestSummary())

	if !contextTestSummary {
		t.Error("WithTestSummary should set enabledTestSummary to true")
	}

	if testSummaryObject == nil {
		t.Error("WithTestSummary should initialize testSummary")
	}
}

func TestNewTestSummary(t *testing.T) {
	// Test newTestSummary function by accessing it through WithTestSummary
	summary := newTestSummary()
	if summary == nil {
		t.Error("newTestSummary should not return nil")
	}

	// Verify it has the expected structure (empty initially)
	colorConfig := color.New()
	summaryStr := summary.String(colorConfig)
	if summaryStr == "" {
		t.Error("newTestSummary should create a valid summary object")
	}
}

// testLogReplacer implements LogReplacer interface for testing.
type testLogReplacer struct {
	replacer *strings.Replacer
}

func (t *testLogReplacer) ReplaceAll(s string) string {
	return t.replacer.Replace(s)
}

func TestSetLogReplacer(t *testing.T) {
	// Test SetLogReplacer function
	originalReplacer := &testLogReplacer{
		replacer: strings.NewReplacer("old", "new"),
	}

	Run(func(r Reporter) {
		// Test the SetLogReplacer function
		SetLogReplacer(r, originalReplacer)

		// Verify it was set by logging something that should be replaced
		r.Log("This contains old text")

		// Access internal state to verify replacer was set
		rep, ok := r.(*reporter)
		if ok && rep.logs != nil {
			logs := rep.logs.all()
			if len(logs) > 0 {
				// Check if the replacement happened
				lastLog := logs[len(logs)-1]
				if !strings.Contains(lastLog, "new") || strings.Contains(lastLog, "old") {
					t.Errorf("Log replacement failed: expected 'new' instead of 'old', got: %s", lastLog)
				}
			}
		}
	})
}

func TestReporterSetLogReplacer(t *testing.T) {
	// Test the internal setLogReplacer method
	rep := newReporter()
	replacer := &testLogReplacer{
		replacer: strings.NewReplacer("hello", "hi", "world", "earth"),
	}

	// Call the internal setLogReplacer method via the public function
	SetLogReplacer(rep, replacer)

	// Add a log that should be replaced
	rep.Log("hello world")

	// Verify the replacement occurred
	logs := rep.getLogs().all()
	if len(logs) == 0 {
		t.Error("Expected at least one log entry")
		return
	}

	lastLog := logs[len(logs)-1]
	if !strings.Contains(lastLog, "hi earth") {
		t.Errorf("Expected log to contain 'hi earth', got: %s", lastLog)
	}
}

func TestSkipColor(t *testing.T) {
	// Test skipColor function by creating scenarios where it's called
	rep := newReporter()

	// Test with color enabled
	colorConfig := color.New()
	colorConfig.SetEnabled(true)
	rep.context = &testContext{colorConfig: colorConfig}
	skipColorObj := rep.skipColor()
	if skipColorObj == nil {
		t.Error("skipColor should return a color object")
	}

	// Test with color disabled
	colorConfigDisabled := color.New()
	colorConfigDisabled.SetEnabled(false)
	rep.context.colorConfig = colorConfigDisabled
	skipColorObj = rep.skipColor()
	if skipColorObj == nil {
		t.Error("skipColor should return a color object even with color disabled")
	}
}

func TestPassColor(t *testing.T) {
	// Test passColor function
	rep := newReporter()

	// Test with color enabled
	colorConfig := color.New()
	colorConfig.SetEnabled(true)
	rep.context = &testContext{colorConfig: colorConfig}
	passColorObj := rep.passColor()
	if passColorObj == nil {
		t.Error("passColor should return a color object")
	}

	// Test with color disabled
	colorConfigDisabled := color.New()
	colorConfigDisabled.SetEnabled(false)
	rep.context.colorConfig = colorConfigDisabled
	passColorObj = rep.passColor()
	if passColorObj == nil {
		t.Error("passColor should return a color object even with color disabled")
	}
}

func TestFailColor(t *testing.T) {
	// Test failColor function
	rep := newReporter()

	// Test with color enabled
	colorConfig := color.New()
	colorConfig.SetEnabled(true)
	rep.context = &testContext{colorConfig: colorConfig}
	failColorObj := rep.failColor()
	if failColorObj == nil {
		t.Error("failColor should return a color object")
	}

	// Test with color disabled
	colorConfigDisabled := color.New()
	colorConfigDisabled.SetEnabled(false)
	rep.context.colorConfig = colorConfigDisabled
	failColorObj = rep.failColor()
	if failColorObj == nil {
		t.Error("failColor should return a color object even with color disabled")
	}
}

func TestReporterRun(t *testing.T) {
	// Test the Run method on reporter to increase coverage
	var parentReporter *reporter
	var runCount int

	Run(func(r Reporter) {
		rep, ok := r.(*reporter)
		if !ok {
			t.Fatal("reporter is not of expected type")
		}
		parentReporter = rep

		// Test Run method coverage
		r.Run("test-run", func(child Reporter) {
			runCount++
			child.Log("child run test")
		})

		// Test with another run
		r.Run("test-run-2", func(child Reporter) {
			runCount++
			child.Log("child run test 2")
		})
	})

	if runCount != 2 {
		t.Errorf("Expected 2 run calls, got %d", runCount)
	}

	// Verify children were created
	children := parentReporter.getChildren()
	if len(children) < 2 {
		t.Errorf("Expected at least 2 children, got %d", len(children))
	}
}

func TestPrintTestSummary(t *testing.T) {
	// Test printTestSummary method by creating conditions that trigger it
	var summaryPrinted bool

	Run(func(r Reporter) {
		rep, ok := r.(*reporter)
		if !ok {
			t.Fatal("reporter is not of expected type")
		}

		// Add some test results to the summary
		r.Run("pass-test", func(child Reporter) {
			child.Log("this test passes")
			// Don't fail, so it counts as passed
		})

		r.Run("fail-test", func(child Reporter) {
			child.Log("this test fails")
			child.Fail()
		})

		r.Run("skip-test", func(child Reporter) {
			child.Skip("this test is skipped")
		})

		// Call printTestSummary if context has test summary enabled
		if rep.context != nil && rep.context.testSummary != nil {
			rep.printTestSummary()
			summaryPrinted = true
		}
	}, WithTestSummary())

	if !summaryPrinted {
		t.Error("printTestSummary should have been called with WithTestSummary")
	}
}

func TestReporterParallel(t *testing.T) {
	// Test Parallel method to increase coverage
	Run(func(r Reporter) {
		// Test Parallel method - should not panic
		r.Parallel()

		// Test that it can be called multiple times
		r.Parallel()

		// If we get here, the Parallel method worked correctly
	})
}

func TestTestSummaryColorMethods(t *testing.T) {
	// Test color methods in test summary
	summary := newTestSummary()

	// Test that the summary object can format output with different color settings
	colorConfig := color.New()
	colorConfig.SetEnabled(true)
	output1 := summary.String(colorConfig) // with color enabled
	if output1 == "" {
		t.Error("Summary should produce output")
	}

	colorConfigDisabled := color.New()
	colorConfigDisabled.SetEnabled(false)
	output2 := summary.String(colorConfigDisabled) // with color disabled
	if output2 == "" {
		t.Error("Summary should produce output even with color disabled")
	}
}
