package reporter

import (
	"fmt"
	"sync"

	"github.com/scenarigo/scenarigo/color"
)

type testSummary struct {
	mu           sync.Mutex
	passedCount  int
	failed       []string
	skippedCount int
}

func newTestSummary() *testSummary {
	return &testSummary{
		mu:           sync.Mutex{},
		passedCount:  0,
		failed:       []string{},
		skippedCount: 0,
	}
}

func (s *testSummary) append(testFileRelPath string, r Reporter) {
	if s == nil {
		return
	}
	testResultString := TestResultString(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	switch testResultString {
	case TestResultPassed.String():
		s.passedCount++
	case TestResultFailed.String():
		s.failed = append(s.failed, testFileRelPath)
	case TestResultSkipped.String():
		s.skippedCount++
	default: // Do nothing
	}
}

// String converts testSummary to the string like below.
// 11 tests run: 9 passed, 2 failed, 0 skipped
//
// Failed tests:
//   - scenarios/scenario1.yaml
//   - scenarios/scenario2.yaml
func (s *testSummary) String(colorConfig *color.Config) string {
	totalText := fmt.Sprintf("%d tests run", s.passedCount+len(s.failed)+s.skippedCount)
	passedText := s.passColor(colorConfig).Sprintf("%d passed", s.passedCount)
	failedText := s.failColor(colorConfig).Sprintf("%d failed", len(s.failed))
	skippedText := s.skipColor(colorConfig).Sprintf("%d skipped", s.skippedCount)
	failedFiles := s.failColor(colorConfig).Sprint(s.failedFiles())
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

func (s *testSummary) passColor(colorConfig *color.Config) *color.Color {
	if colorConfig != nil {
		return colorConfig.Green()
	}
	return color.New().Green()
}

func (s *testSummary) failColor(colorConfig *color.Config) *color.Color {
	if colorConfig != nil {
		return colorConfig.HiRed()
	}
	return color.New().HiRed()
}

func (s *testSummary) skipColor(colorConfig *color.Config) *color.Color {
	if colorConfig != nil {
		return colorConfig.Yellow()
	}
	return color.New().Yellow()
}
