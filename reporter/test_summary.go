package reporter

import (
	"fmt"
	"sync"

	"github.com/fatih/color"
)

type testSummary struct {
	mu           sync.Mutex
	enabledColor bool
	passed       []string
	failed       []string
	skipped      []string
}

func newTestSummary(enabledColor bool) testSummary {
	return testSummary{
		mu:           sync.Mutex{},
		enabledColor: enabledColor,
		passed:       []string{},
		failed:       []string{},
		skipped:      []string{},
	}
}

func (s *testSummary) append(testFileRelPath string, testResultString string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch testResultString {
	case TestResultPassed.String():
		s.passed = append(s.passed, testFileRelPath)
	case TestResultFailed.String():
		s.failed = append(s.failed, testFileRelPath)
	case TestResultSkipped.String():
		s.skipped = append(s.skipped, testFileRelPath)
	default: // Do nothing
	}
}

// String converts testSummary to the string like below.
// 11 tests run: 9 passed, 2 failed, 0 skipped
//
// Failed tests:
//   - scenarios/scenario1.yaml
//   - scenarios/scenario2.yaml
func (s *testSummary) String() string {
	totalText := fmt.Sprintf("%d tests run", len(s.passed)+len(s.failed)+len(s.skipped))
	passedText := s.passColor().Sprintf("%d passed", len(s.passed))
	failedText := s.failColor().Sprintf("%d failed", len(s.failed))
	skippedText := s.skipColor().Sprintf("%d skipped", len(s.skipped))
	failedFiles := s.failColor().Sprintf(s.failedFiles())
	return fmt.Sprintf(
		"\n%s: %s, %s, %s\n\n%s",
		totalText, passedText, failedText, skippedText, failedFiles,
	)
}

func (s *testSummary) failedFiles() string {
	if len(s.failed) == 0 {
		return ""
	}

	result := ""

	for _, f := range s.failed {
		if result == "" {
			result = "Failed tests:\n"
		}
		result += fmt.Sprintf("\t- %s\n", f)
	}
	result += "\n"

	return result
}

func (s *testSummary) passColor() *color.Color {
	if !s.enabledColor {
		return color.New()
	}
	return color.New(color.FgGreen)
}

func (s *testSummary) failColor() *color.Color {
	if !s.enabledColor {
		return color.New()
	}
	return color.New(color.FgHiRed)
}

func (s *testSummary) skipColor() *color.Color {
	if !s.enabledColor {
		return color.New()
	}
	return color.New(color.FgYellow)
}
