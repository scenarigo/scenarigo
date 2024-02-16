package scenarigo

import (
	"fmt"
	"sync"

	"github.com/zoncoen/scenarigo/reporter"
)

type testSummary struct {
	mu      sync.Mutex
	passed  []string
	failed  []string
	skipped []string
}

func (s *testSummary) add(testFileRelPath string, r reporter.Reporter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch reporter.TestResultString(r) {
	case reporter.TestResultPassed.String():
		s.passed = append(s.passed, testFileRelPath)
	case reporter.TestResultFailed.String():
		s.failed = append(s.failed, testFileRelPath)
	case reporter.TestResultSkipped.String():
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
//
// TODO(kyu08): Add UT.
func (s *testSummary) String() string {
	return fmt.Sprintf(
		"\n%d tests run: %d passed, %d failed, %d skipped\n\n%s",
		len(s.passed)+len(s.failed)+len(s.skipped), len(s.passed), len(s.failed), len(s.skipped), s.failedFiles(),
	)
}

// TODO(kyu08): Add UT.
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
