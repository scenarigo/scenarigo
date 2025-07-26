// Package reporter provides test result reporters.
// It is intended to be used in scenarigo.
package reporter

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/cenkalti/backoff/v4"
	"github.com/fatih/color"
)

// teardownFunc represents a named teardown function.
type teardownFunc struct {
	name string
	f    func(r Reporter)
}

// cleanupFunc represents an unnamed cleanup function.
type cleanupFunc func()

// A Reporter is something that can be used to report test results.
type Reporter interface {
	Name() string
	Fail()
	Failed() bool
	FailNow()
	Log(args ...any)
	Logf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Skip(args ...any)
	Skipf(format string, args ...any)
	SkipNow()
	Skipped() bool
	Parallel()
	Run(name string, f func(r Reporter)) bool
	Teardown(name string, f func(r Reporter))
	Cleanup(f func())

	runWithRetry(string, func(t Reporter), RetryPolicy) bool
	setNoFailurePropagation()
	setLogReplacer(LogReplacer)

	// for test reports
	getName() string
	getDuration() time.Duration
	getLogs() *logRecorder
	getChildren() []Reporter
	isRoot() bool

	// for test summary
	printTestSummary()
}

// Run runs f with new Reporter which applied opts.
// It reports whether f succeeded.
func Run(f func(r Reporter), opts ...Option) bool {
	r := run(f, opts...)

	// print global errors (e.g., invalid config)
	if (r.Failed() && !r.noFailurePropagation) || r.context.verbose {
		c := r.passColor()
		if r.Failed() {
			c = r.failColor()
		} else if r.Skipped() {
			c = r.skipColor()
		}
		for _, l := range r.logs.all() {
			r.context.printf("%s\n", c.Sprint(l))
		}
	}

	r.printTestSummary()
	return !r.Failed()
}

func run(f func(r Reporter), opts ...Option) *reporter {
	r := newReporter()
	r.context = newTestContext(opts...)
	go r.run(f)
	<-r.done
	return r
}

// NoFailurePropagation prevents propagation of the failure to the parent.
func NoFailurePropagation(r Reporter) {
	r.setNoFailurePropagation()
}

// SetLogReplacer sets a replacer to modify log outputs.
func SetLogReplacer(r Reporter, rep LogReplacer) {
	r.setLogReplacer(rep)
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
	durationMeasurer testDurationMeasurer
	children         []*reporter
	teardowns        []teardownFunc
	cleanups         []cleanupFunc
	inCleanup        bool // Flag to indicate if cleanup is running

	barrier chan bool // To signal parallel subtests they may start.
	done    chan bool // To signal a test is done.

	testing              bool
	retryPolicy          RetryPolicy
	retryable            bool
	noFailurePropagation bool
}

func newReporter() *reporter {
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
	if r.parent != nil && !r.retryable && !r.noFailurePropagation {
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
func (r *reporter) Log(args ...any) {
	r.logs.log(fmt.Sprint(args...))
}

// Logf formats its arguments according to the format, analogous to fmt.Printf, and
// records the text in the log.
// The text will be printed only if the test fails or the --verbose flag is set.
func (r *reporter) Logf(format string, args ...any) {
	r.logs.log(fmt.Sprintf(format, args...))
}

// Error is equivalent to Log followed by Fail.
func (r *reporter) Error(args ...any) {
	r.Fail()
	r.logs.error(fmt.Sprint(args...))
}

// Errorf is equivalent to Logf followed by Fail.
func (r *reporter) Errorf(format string, args ...any) {
	r.Fail()
	r.logs.error(fmt.Sprintf(format, args...))
}

// Fatal is equivalent to Log followed by FailNow.
func (r *reporter) Fatal(args ...any) {
	r.Error(args...)
	runtime.Goexit()
}

// Fatalf is equivalent to Logf followed by FailNow.
func (r *reporter) Fatalf(format string, args ...any) {
	r.Errorf(format, args...)
	runtime.Goexit()
}

// Skip is equivalent to Log followed by SkipNow.
func (r *reporter) Skip(args ...any) {
	r.logs.skip(fmt.Sprint(args...))
	r.SkipNow()
}

// Skipf is equivalent to Logf followed by SkipNow.
func (r *reporter) Skipf(format string, args ...any) {
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
		if r.retryPolicy != nil {
			r.m.Unlock()
			return
		}
		panic("reporter: Reporter.Parallel called multiple times")
	}
	r.isParallel = true
	r.durationMeasurer.stop()
	defer r.durationMeasurer.start()
	r.m.Unlock()

	// Retry attempts can not be executed in parallel.
	if r.retryable {
		r.parent.Parallel()
		return
	}

	if r.context.verbose {
		r.context.printf("=== PAUSE %s\n", r.goTestName)
	}
	r.done <- true     // Release calling test.
	<-r.parent.barrier // Wait for the parent test to complete.
	r.context.waitParallel()

	if r.context.verbose {
		r.context.printf("=== CONT  %s\n", r.goTestName)
	}
}

func (r *reporter) printTestSummary() {
	if !r.context.enabledTestSummary {
		return
	}
	_, _ = r.context.print(r.context.testSummary.String(r.context.noColor)) //nolint:forbidigo
}

func AppendReporter(dst Reporter, src Reporter) {
	d, dok := dst.(*reporter)
	s, sok := src.(*reporter)
	if dok && sok {
		d.appendChildren(s.children...)
	}
}

func (r *reporter) appendChildren(children ...*reporter) {
	r.m.Lock()
	r.children = append(r.children, children...)
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
	if r.inCleanup {
		panic("reporter: Run called during cleanup")
	}
	return r.runWithRetry(name, f, nil)
}

func (r *reporter) runWithRetry(name string, f func(t Reporter), policy RetryPolicy) bool {
	if !r.context.matcher.match(r.goTestName, rewrite(name)) {
		return true
	}
	child := r.spawn(name)
	child.retryPolicy = policy
	if r.context.verbose {
		r.context.printf("=== RUN   %s\n", child.goTestName)
	}
	go child.run(f)
	<-child.done
	r.appendChildren(child)
	if r.isRoot() {
		// Only print report immediately for non-parallel tests
		// Parallel tests will be printed when the parent test completes
		if !child.isParallel {
			printReport(child)
			child.context.testSummary.append(name, child)
		}
	}
	return !child.Failed()
}

func (r *reporter) spawn(name string) *reporter {
	goTestName := rewrite(name)
	if r.goTestName != "" {
		goTestName = fmt.Sprintf("%s/%s", r.goTestName, goTestName)
	}
	child := newReporter()
	child.context = r.context
	child.parent = r
	child.name = name
	child.goTestName = goTestName
	child.depth = r.depth + 1
	child.logs = r.logs.spawn()
	child.durationMeasurer = r.durationMeasurer.spawn()
	child.testing = r.testing
	return child
}

func (r *reporter) run(f func(r Reporter)) {
	stop := r.start()
	defer stop()

	if r.retryPolicy == nil {
		r.runFunc(f)
	} else {
		_, cancel, b, err := r.retryPolicy.Build(context.Background())
		if err != nil {
			r.Fatalf("invalid retry policy: %s", err)
		}
		defer cancel()
		var retried bool
		child, err := backoff.RetryNotifyWithData(func() (*reporter, error) {
			child := r.spawn("retryable")
			child.name = r.name
			child.goTestName = r.goTestName
			child.depth = r.depth
			child.retryable = true
			// Children never run in parallel.
			// See Parallel().
			go child.run(f)
			<-child.done
			if child.Failed() {
				return child, errors.New("failed")
			}
			return child, nil
		}, b, func(err error, d time.Duration) {
			retried = true
			r.Logf("retry after %s", d)
		})
		r.noFailurePropagation = child.noFailurePropagation
		if retried && err != nil {
			r.Error("retry limit exceeded")
		}
		r.logs.append(child.logs)
		r.appendChildren(child.children...)
		if err != nil {
			if child.Failed() {
				r.FailNow()
			}
		}
		if child.Skipped() {
			r.SkipNow()
		}
	}
}

func (r *reporter) runFunc(f func(Reporter)) {
	var finished bool
	defer func() {
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
	}()
	f(r)
	finished = true
}

func (r *reporter) start() func() {
	r.durationMeasurer.start()
	return func() {
		r.durationMeasurer.stop()
		err := recover()
		if err != nil {
			if !r.Failed() && !r.Skipped() {
				r.Error(err)
				r.Error(string(debug.Stack()))
			}
		}

		// Collect subtests which are running parallel (only original subtests, not teardowns).
		subtests := make([]<-chan bool, 0, len(r.children))
		// No need to wait for retry attempts.
		// They never run in parallel.
		if r.retryPolicy == nil {
			for _, child := range r.children {
				if child.isParallel {
					done := make(chan bool)
					go func() {
						<-child.done
						// Print reports for parallel subtests after they complete
						if r.isRoot() {
							if child.isParallel {
								printReport(child)
								child.context.testSummary.append(child.name, child)
							}
						}
						close(done)
					}()
					subtests = append(subtests, done)
				}
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

		// Run teardown functions in reverse order (LIFO) as subtests.
		// Each teardown runs as a normal subtest and can contain parallel subtests.
		for i := len(r.teardowns) - 1; i >= 0; i-- {
			teardown := r.teardowns[i]
			func() {
				defer func() {
					if teardownErr := recover(); teardownErr != nil {
						r.Errorf("panic in teardown: %v\n%s", teardownErr, debug.Stack())
					}
				}()
				// Run teardown as a subtest using the existing Run method
				r.Run(teardown.name, teardown.f)
			}()
		}

		// Run cleanup functions in reverse order (LIFO) directly without subtests.
		if len(r.cleanups) > 0 {
			r.inCleanup = true
			for i := len(r.cleanups) - 1; i >= 0; i-- {
				cleanup := r.cleanups[i]
				func() {
					defer func() {
						if cleanupErr := recover(); cleanupErr != nil {
							r.Errorf("panic in cleanup: %v\n%s", cleanupErr, debug.Stack())
						}
					}()
					cleanup()
				}()
			}
			r.inCleanup = false
		}

		r.done <- true
	}
}

func printReport(r *reporter) {
	results := collectOutput(r)
	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(r)
		sb.WriteString("\n")
	}
	if r.Failed() && !r.testing {
		sb.WriteString(r.failColor().Sprintln("FAIL"))
	}
	r.context.print(sb.String()) //nolint:forbidigo
}

func collectOutput(r *reporter) []string {
	var results []string
	// For parallel tests, always show logs in verbose mode, regardless of failure status
	shouldShowLogs := (r.Failed() && !r.noFailurePropagation) || r.context.verbose
	if shouldShowLogs {
		prefix := strings.Repeat("    ", r.depth-1)
		status := "PASS"
		c := r.passColor()
		if r.Failed() {
			status = "FAIL"
			c = r.failColor()
		} else if r.Skipped() {
			status = "SKIP"
			c = r.skipColor()
		}
		results = []string{
			c.Sprintf("%s--- %s: %s (%.2fs)", prefix, status, r.goTestName, r.durationMeasurer.getDuration().Seconds()),
		}
		for _, l := range r.logs.all() {
			padding := fmt.Sprintf("%s    ", prefix)
			results = append(results, pad(l, padding))
		}
	}
	for _, child := range r.children {
		results = append(results, collectOutput(child)...)
	}
	if r.depth == 1 && !r.testing {
		if r.Failed() {
			results = append(results,
				//nolint:dupword
				r.failColor().Sprintf("FAIL\nFAIL\t%s\t%.3fs", r.goTestName, r.durationMeasurer.getDuration().Seconds()),
			)
		} else {
			if r.context.verbose {
				results = append(results, r.passColor().Sprint("PASS"))
			}
			results = append(results,
				r.passColor().Sprintf("ok  \t%s\t%.3fs", r.goTestName, r.durationMeasurer.getDuration().Seconds()),
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

func (r *reporter) setNoFailurePropagation() {
	r.noFailurePropagation = true
}

func (r *reporter) setLogReplacer(rep LogReplacer) {
	r.logs.setReplacer(rep)
}

func (r *reporter) getName() string {
	return r.name
}

func (r *reporter) getDuration() time.Duration {
	return r.durationMeasurer.getDuration()
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

func (r *reporter) passColor() *color.Color {
	if r.context.noColor {
		return color.New()
	}
	return color.New(color.FgGreen)
}

func (r *reporter) failColor() *color.Color {
	if r.context.noColor {
		return color.New()
	}
	return color.New(color.FgHiRed)
}

func (r *reporter) skipColor() *color.Color {
	if r.context.noColor {
		return color.New()
	}
	return color.New(color.FgYellow)
}

// Teardown registers a named function to be called when all parallel subtests complete.
// The teardown functions are called in reverse order of registration (LIFO) as subtests.
func (r *reporter) Teardown(name string, f func(r Reporter)) {
	r.m.Lock()
	defer r.m.Unlock()
	r.teardowns = append(r.teardowns, teardownFunc{name: name, f: f})
}

// Cleanup registers a function to be called when the test completes.
// This is similar to testing.T.Cleanup() in Go's standard testing package.
// The cleanup functions are called in reverse order of registration (LIFO).
func (r *reporter) Cleanup(f func()) {
	r.m.Lock()
	defer r.m.Unlock()
	r.cleanups = append(r.cleanups, cleanupFunc(f))
}
