package testutil

import (
	"regexp"
	"sort"
	"strings"
)

var (
	contPattern    *regexp.Regexp
	passPattern    *regexp.Regexp
	failPattern    *regexp.Regexp
	okPattern      *regexp.Regexp
	failEndPattern *regexp.Regexp
)

func init() {
	contPattern = regexp.MustCompile(`^=== CONT\s+`)
	passPattern = regexp.MustCompile(`^--- PASS:\s+(.+?)\s+`)
	failPattern = regexp.MustCompile(`^--- FAIL:\s+(.+?)\s+`)
	okPattern = regexp.MustCompile(`^ok\s+`)
	failEndPattern = regexp.MustCompile(`^FAIL$`)
}

type testBlockInfo struct {
	name  string
	block []string
}

// extractTestName extracts the base test name from a full test name (removes subtest suffix).
func extractBaseTestName(testName string) string {
	if idx := strings.Index(testName, "/"); idx >= 0 {
		return testName[:idx]
	}
	return testName
}

// collectTestBlock collects all lines belonging to a test result block.
func collectTestBlock(lines []string, startIdx int, isFailure bool) ([]string, int) {
	block := []string{lines[startIdx]}
	i := startIdx + 1

	for i < len(lines) {
		block = append(block, lines[i])
		if isFailure && failEndPattern.MatchString(lines[i]) {
			i++
			// For failures, collect the additional FAIL lines
			// Format: FAIL / FAIL 	testname	0.000s / FAIL
			for count := 0; count < 2 && i < len(lines) && strings.HasPrefix(lines[i], "FAIL"); count++ {
				block = append(block, lines[i])
				i++
			}
			break
		} else if !isFailure && okPattern.MatchString(lines[i]) {
			i++
			break
		}
		i++
	}

	return block, i
}

// parseTestResult extracts test name and failure status from a test result line.
func parseTestResult(line string) (string, bool, bool) {
	if passMatch := passPattern.FindStringSubmatch(line); passMatch != nil {
		return passMatch[1], false, true
	}
	if failMatch := failPattern.FindStringSubmatch(line); failMatch != nil {
		return failMatch[1], true, true
	}
	return "", false, false
}

// parseOkLine extracts test name from a standalone ok line.
func parseOkLine(line string) (string, bool) {
	if okPattern.MatchString(line) {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			return parts[1], true
		}
	}
	return "", false
}

// sortTestBlocks sorts test blocks by test name.
func sortTestBlocks(blocks []testBlockInfo) {
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].name < blocks[j].name
	})
}

// processStandaloneOkLine processes standalone ok lines and collects parallel test blocks.
func processStandaloneOkLine(lines []string, i int, isParallelTest func(string) bool, result []string) ([]string, int) {
	testName, isOk := parseOkLine(lines[i])
	if !isOk || !isParallelTest(testName) {
		return append(result, lines[i]), i + 1
	}

	// Check if there are test blocks following
	hasTestBlocks := false
	for j := i + 1; j < len(lines); j++ {
		if _, _, isTest := parseTestResult(lines[j]); isTest {
			hasTestBlocks = true
			break
		}
	}

	if !hasTestBlocks {
		// Only standalone ok lines
		blocks := collectStandaloneOkLines(lines, i, isParallelTest)
		sortTestBlocks(blocks)
		for _, block := range blocks {
			result = append(result, block.block...)
		}

		// Update index
		nextIdx := i
		for _, block := range blocks {
			nextIdx += len(block.block)
		}
		return result, nextIdx
	}

	// If there are test blocks following, fall through
	return append(result, lines[i]), i + 1
}

// collectRemainingParallelBlocks collects all remaining parallel test blocks from lines.
func collectRemainingParallelBlocks(lines []string, startIdx int, isParallelTest func(string) bool, processedTests map[string]bool) ([]testBlockInfo, []string) {
	parallelBlocks := []testBlockInfo{}
	remainingLines := []string{}
	j := startIdx

	for j < len(lines) {
		// Skip standalone ok lines for parallel tests
		if testName2, isOk := parseOkLine(lines[j]); isOk {
			switch {
			case isParallelTest(testName2) && !processedTests[testName2]:
				parallelBlocks = append(parallelBlocks, testBlockInfo{testName2, []string{lines[j]}})
				processedTests[testName2] = true
				j++
				continue
			case !isParallelTest(testName2):
				// Non-parallel ok line, add to remaining
				remainingLines = append(remainingLines, lines[j])
				j++
				continue
			default:
				// Already processed parallel ok line, skip
				j++
				continue
			}
		}

		// Check for test result line
		testName2, isFailure2, isTestResult2 := parseTestResult(lines[j])
		if isTestResult2 {
			// Collect the test block
			block2, nextIdx2 := collectTestBlock(lines, j, isFailure2)

			if isParallelTest(testName2) && !processedTests[testName2] {
				parallelBlocks = append(parallelBlocks, testBlockInfo{testName2, block2})
				processedTests[testName2] = true
			} else if !isParallelTest(testName2) {
				// Non-parallel test block, add to remaining
				remainingLines = append(remainingLines, block2...)
			}
			// If already processed parallel test, just skip
			j = nextIdx2
		} else {
			// Not a test result, add to remaining
			remainingLines = append(remainingLines, lines[j])
			j++
		}
	}

	return parallelBlocks, remainingLines
}

// NormalizeParallelOutput normalizes parallel test output to make it deterministic.
// It sorts:
// 1. Consecutive === CONT lines
// 2. Test result blocks (from "--- PASS/FAIL: {testname}" to "ok {testname}" or "FAIL")
// Only test names explicitly passed in the testnames parameter are treated as parallel tests.
func NormalizeParallelOutput(output string, testnames []string) string {
	lines := strings.Split(output, "\n")

	// Create a set of test names for quick lookup
	testnameSet := make(map[string]bool)
	for _, name := range testnames {
		testnameSet[name] = true
	}

	// Helper to check if a test name is parallel
	isParallelTest := func(testName string) bool {
		return testnameSet[extractBaseTestName(testName)]
	}

	result := make([]string, 0, len(lines))
	processedTests := make(map[string]bool) // Track which parallel tests have been processed
	i := 0
	for i < len(lines) {
		line := lines[i]

		// Handle RUN/PAUSE lines normally
		if strings.HasPrefix(line, "=== RUN") || strings.HasPrefix(line, "=== PAUSE") {
			result = append(result, line)
			i++
			continue
		}

		// Handle CONT lines
		if contPattern.MatchString(line) {
			contLines := []string{}
			for i < len(lines) && contPattern.MatchString(lines[i]) {
				contLines = append(contLines, lines[i])
				i++
			}
			sort.Strings(contLines)
			result = append(result, contLines...)
			continue
		}

		// Handle standalone ok line
		if _, isOk := parseOkLine(line); isOk {
			result, i = processStandaloneOkLine(lines, i, isParallelTest, result)
			continue
		}

		// Handle test result (--- PASS/FAIL)
		testName, isFailure, isTestResult := parseTestResult(line)
		if !isTestResult {
			// Not a test result, just add it
			result = append(result, line)
			i++
			continue
		}

		// Collect the test block
		testBlock, nextIdx := collectTestBlock(lines, i, isFailure)
		i = nextIdx

		if isParallelTest(testName) {
			// Check if this test has already been processed
			if processedTests[testName] {
				// Skip, already processed as part of a parallel block group
				continue
			}

			// Collect ALL remaining parallel test blocks from the entire rest of the input
			initialBlocks := []testBlockInfo{{testName, testBlock}}
			processedTests[testName] = true

			// Collect remaining parallel blocks
			parallelBlocks, remainingLines := collectRemainingParallelBlocks(lines, i, isParallelTest, processedTests)
			parallelBlocks = append(initialBlocks, parallelBlocks...)

			// Sort parallel blocks and add them
			sortTestBlocks(parallelBlocks)
			for _, block := range parallelBlocks {
				result = append(result, block.block...)
			}

			// Process remaining lines recursively
			if len(remainingLines) > 0 {
				remainingOutput := strings.Join(remainingLines, "\n")
				remainingNormalized := NormalizeParallelOutput(remainingOutput, testnames)
				if remainingNormalized != "" {
					result = append(result, strings.Split(remainingNormalized, "\n")...)
				}
			}

			// We've processed everything, break out of the main loop
			break
		} else {
			// Non-parallel test, just add it
			result = append(result, testBlock...)
		}
	}

	return strings.Join(result, "\n")
}

// collectStandaloneOkLines collects consecutive standalone ok lines for parallel tests.
func collectStandaloneOkLines(lines []string, startIdx int, isParallelTest func(string) bool) []testBlockInfo {
	blocks := []testBlockInfo{}

	// Add first ok line
	if testName, isOk := parseOkLine(lines[startIdx]); isOk {
		blocks = append(blocks, testBlockInfo{testName, []string{lines[startIdx]}})
	}

	// Look ahead for more ok lines
	j := startIdx + 1
	for j < len(lines) {
		if testName, isOk := parseOkLine(lines[j]); isOk {
			if isParallelTest(testName) {
				blocks = append(blocks, testBlockInfo{testName, []string{lines[j]}})
				j++
			} else {
				break
			}
		} else {
			break
		}
	}

	return blocks
}
