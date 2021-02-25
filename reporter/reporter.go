// Package reporter provides test result reporters.
// It is intended to be used in scenarigo.
package reporter

import (
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

// A Reporter is something that can be used to report test results.
type Reporter interface {
	Name() string
	Fail()
	Failed() bool
	FailNow()
	Log(args ...interface{})
	Logf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Skip(args ...interface{})
	Skipf(format string, args ...interface{})
	SkipNow()
	Skipped() bool
	Parallel()
	Run(name string, f func(r Reporter)) bool

	// for test reports
	getName() string
	getDuration() TestDuration
	getLogs() *logRecorder
	getChildren() []Reporter
	isRoot() bool
}

// Run runs f with new Reporter which applied opts.
// It reports whether f succeeded.
func Run(f func(r Reporter), opts ...Option) bool {
	r := run(f, opts...)
	return !r.Failed()
}

func run(f func(r Reporter), opts ...Option) *reporter {
	r := new()
	r.context = newTestContext(opts...)
	go r.run(f)
	<-r.done
	return r
}

// reporter is an implementation of Reporter that
// records its mutations for later inspection in tests.
type reporter struct {
	m                sync.Mutex
	context          *testContext
	parent           *reporter
	name             string
	goTestName       string
	depth            int // Nesting depth of test.
	failed           int32
	skipped          int32
	isParallel       bool
	logs             *logRecorder
	durationMeasurer *durationMeasurer
	children         []*reporter

	barrier chan bool // To signal parallel subtests they may start.
	done    chan bool // To signal a test is done.

	zeroDuration bool // for testing
}

type logRecorder struct {
	m         sync.Mutex
	strs      []string
	infoIdxs  []int
	errorIdxs []int
	skipIdx   *int
}

func (r *logRecorder) log(s string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.strs = append(r.strs, s)
	r.infoIdxs = append(r.infoIdxs, len(r.strs)-1)
}

func (r *logRecorder) error(s string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.strs = append(r.strs, s)
	r.errorIdxs = append(r.errorIdxs, len(r.strs)-1)
}

func (r *logRecorder) skip(s string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.strs = append(r.strs, s)
	i := len(r.strs) - 1
	r.skipIdx = &i
}

func (r *logRecorder) all() []string {
	r.m.Lock()
	defer r.m.Unlock()
	strs := make([]string, len(r.strs))
	for i, str := range r.strs {
		strs[i] = str
	}
	return strs
}

func (r *logRecorder) infoLogs() []string {
	r.m.Lock()
	defer r.m.Unlock()
	ignore := map[int]struct{}{}
	for _, i := range r.errorIdxs {
		ignore[i] = struct{}{}
	}
	if r.skipIdx != nil {
		ignore[*r.skipIdx] = struct{}{}
	}
	strs := make([]string, 0, len(r.strs)-len(ignore))
	for i, str := range r.strs {
		if _, ok := ignore[i]; ok {
			continue
		}
		strs = append(strs, str)
	}
	if len(strs) == 0 {
		return nil
	}
	return strs
}

func (r *logRecorder) errorLogs() []string {
	r.m.Lock()
	defer r.m.Unlock()
	strs := make([]string, len(r.errorIdxs))
	for i, j := range r.errorIdxs {
		strs[i] = r.strs[j]
	}
	if len(strs) == 0 {
		return nil
	}
	return strs
}

func (r *logRecorder) skipLog() *string {
	r.m.Lock()
	defer r.m.Unlock()
	if r.skipIdx == nil {
		return nil
	}
	return &r.strs[*r.skipIdx]
}

func new() *reporter {
	return &reporter{
		logs:             &logRecorder{},
		durationMeasurer: &durationMeasurer{},
		barrier:          make(chan bool),
		done:             make(chan bool),
	}
}

// Name returns the name of the running test.
func (r *reporter) Name() string {
	return r.goTestName
}

// Fail marks the function as having failed but continues execution.
func (r *reporter) Fail() {
	if r.parent != nil {
		r.parent.Fail()
	}
	atomic.StoreInt32(&r.failed, 1)
}

// Failed reports whether the function has failed.
func (r *reporter) Failed() bool {
	return atomic.LoadInt32(&r.failed) > 0
}

// FailNow marks the function as having failed and stops its execution
// by calling runtime.Goexit (which then runs all deferred calls in the
// current goroutine).
func (r *reporter) FailNow() {
	r.Fail()
	runtime.Goexit()
}

// Log formats its arguments using default formatting, analogous to fmt.Print,
// and records the text in the log.
// The text will be printed only if the test fails or the --verbose flag is set.
func (r *reporter) Log(args ...interface{}) {
	r.logs.log(fmt.Sprint(args...))
}

// Logf formats its arguments according to the format, analogous to fmt.Printf, and
// records the text in the log.
// The text will be printed only if the test fails or the --verbose flag is set.
func (r *reporter) Logf(format string, args ...interface{}) {
	r.logs.log(fmt.Sprintf(format, args...))
}

// Error is equivalent to Log followed by Fail.
func (r *reporter) Error(args ...interface{}) {
	r.Fail()
	r.logs.error(fmt.Sprint(args...))
}

// Errorf is equivalent to Logf followed by Fail.
func (r *reporter) Errorf(format string, args ...interface{}) {
	r.Fail()
	r.logs.error(fmt.Sprintf(format, args...))
}

// Fatal is equivalent to Log followed by FailNow.
func (r *reporter) Fatal(args ...interface{}) {
	r.Error(args...)
	runtime.Goexit()
}

// Fatalf is equivalent to Logf followed by FailNow.
func (r *reporter) Fatalf(format string, args ...interface{}) {
	r.Errorf(format, args...)
	runtime.Goexit()
}

// Skip is equivalent to Log followed by SkipNow.
func (r *reporter) Skip(args ...interface{}) {
	r.logs.skip(fmt.Sprint(args...))
	r.SkipNow()
}

// Skipf is equivalent to Logf followed by SkipNow.
func (r *reporter) Skipf(format string, args ...interface{}) {
	r.logs.skip(fmt.Sprintf(format, args...))
	r.SkipNow()
}

// Skipped reports whether the test was skipped.
func (r *reporter) Skipped() bool {
	return atomic.LoadInt32(&r.skipped) > 0
}

// SkipNow marks the test as having been skipped and stops its execution
// by calling runtime.Goexit.
func (r *reporter) SkipNow() {
	atomic.StoreInt32(&r.skipped, 1)
	runtime.Goexit()
}

// Parallel signals that this test is to be run in parallel with (and only with)
// other parallel tests.
func (r *reporter) Parallel() {
	r.m.Lock()
	if r.isParallel {
		panic("reporter: Reporter.Parallel called multiple times")
	}
	r.isParallel = true
	r.durationMeasurer.stop()
	r.m.Unlock()

	if r.context.verbose {
		r.context.printf("=== PAUSE %s\n", r.goTestName)
	}
	r.done <- true     // Release calling test.
	<-r.parent.barrier // Wait for the parent test to complete.
	r.context.waitParallel()

	if r.context.verbose {
		r.context.printf("=== CONT  %s\n", r.goTestName)
	}
	r.durationMeasurer.start()
}

func (r *reporter) appendChild(child *reporter) {
	r.m.Lock()
	r.children = append(r.children, child)
	r.m.Unlock()
}

// rewrite rewrites a subname to having only printable characters and no white space.
func rewrite(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			b = append(b, '_')
		case !strconv.IsPrint(r):
			s := strconv.QuoteRune(r)
			b = append(b, s[1:len(s)-1]...)
		default:
			b = append(b, string(r)...)
		}
	}
	return string(b)
}

func (r *reporter) isRoot() bool {
	return r.depth == 0
}

// Run runs f as a subtest of r called name.
// It runs f in a separate goroutine and blocks until f returns or calls r.Parallel to become a parallel test.
// Run reports whether f succeeded (or at least did not fail before calling r.Parallel).
//
// Run may be called simultaneously from multiple goroutines,
// but all such calls must return before the outer test function for r returns.
func (r *reporter) Run(name string, f func(t Reporter)) bool {
	goTestName := rewrite(name)
	if r.goTestName != "" {
		goTestName = fmt.Sprintf("%s/%s", r.goTestName, goTestName)
	}
	child := new()
	child.context = r.context
	child.parent = r
	child.name = name
	child.goTestName = goTestName
	child.depth = r.depth + 1
	child.durationMeasurer = r.durationMeasurer.spawn()
	child.zeroDuration = r.zeroDuration
	if r.context.verbose {
		r.context.printf("=== RUN   %s\n", child.goTestName)
	}
	go child.run(f)
	<-child.done
	r.appendChild(child)
	if r.isRoot() {
		print(child)
	}
	return !child.Failed()
}

func (r *reporter) run(f func(r Reporter)) {
	var finished bool
	defer func() {
		r.durationMeasurer.stop()
		err := recover()
		if !finished && err == nil {
			err = errors.New("test executed panic(nil) or runtime.Goexit")
		}
		if err != nil {
			if !r.Failed() && !r.Skipped() {
				r.Error(err)
				r.Error(string(debug.Stack()))
			}
		}

		// Collect subtests which are running parallel.
		subtests := make([]<-chan bool, 0, len(r.children))
		for _, child := range r.children {
			if child.isParallel {
				subtests = append(subtests, child.done)
			}
		}

		if len(subtests) > 0 {
			// Run parallel subtests.
			// Decrease the running count for this test.
			r.context.release()
			// Release the parallel subtests.
			close(r.barrier)
			// Wait for subtests to complete.
			for _, done := range subtests {
				<-done
			}
			if !r.isParallel {
				// Reacquire the count for sequential tests. See comment in Run.
				r.context.waitParallel()
			}
		} else if r.isParallel {
			// Only release the count for this test if it was run as a parallel test.
			r.context.release()
		}

		r.done <- true
	}()

	r.durationMeasurer.start()
	f(r)
	finished = true
}

func print(r *reporter) {
	results := collectOutput(r)
	r.context.printf("%s\n", strings.Join(results, "\n"))
	if r.Failed() {
		r.context.printf("FAIL\n")
	}
}

func collectOutput(r *reporter) []string {
	var results []string
	if r.Failed() || r.context.verbose {
		prefix := strings.Repeat("    ", r.depth-1)
		status := "PASS"
		if r.Failed() {
			status = "FAIL"
		} else if r.Skipped() {
			status = "SKIP"
		}
		results = []string{
			fmt.Sprintf("%s--- %s: %s (%.2fs)", prefix, status, r.goTestName, r.durationMeasurer.getDuration().Seconds()),
		}
		for _, l := range r.logs.all() {
			padding := fmt.Sprintf("%s    ", prefix)
			results = append(results, pad(l, padding))
		}
	}
	for _, child := range r.children {
		results = append(results, collectOutput(child)...)
	}
	if r.depth == 1 {
		if r.Failed() {
			results = append(results,
				fmt.Sprintf("FAIL\nFAIL\t%s\t%.3fs", r.goTestName, r.durationMeasurer.getDuration().Seconds()),
			)
		} else {
			if r.context.verbose {
				results = append(results, "PASS")
			}
			results = append(results,
				fmt.Sprintf("ok  \t%s\t%.3fs", r.goTestName, r.durationMeasurer.getDuration().Seconds()),
			)
		}
	}
	return results
}

func pad(s string, padding string) string {
	s = strings.Trim(s, "\n")
	indent := strings.Repeat(" ", 4)
	var b strings.Builder
	for i, l := range strings.Split(s, "\n") {
		if i == 0 {
			b.WriteString(indent)
		} else {
			b.WriteString("\n" + indent)
		}
		b.WriteString(padding)
		b.WriteString(l)
	}
	return b.String()
}

func (r *reporter) getName() string {
	return r.name
}

func (r *reporter) getDuration() TestDuration {
	if r.zeroDuration {
		return 0
	}
	return TestDuration(r.durationMeasurer.getDuration())
}

func (r *reporter) getLogs() *logRecorder {
	return r.logs
}

func (r *reporter) getChildren() []Reporter {
	children := make([]Reporter, len(r.children))
	for i, child := range r.children {
		children[i] = child
	}
	return children
}
