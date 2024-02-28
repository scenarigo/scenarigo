package reporter

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Option represents an option for test reporter.
type Option func(*testContext)

// WithMaxParallel returns an option to set the number of parallel.
func WithMaxParallel(i int) Option {
	return func(ctx *testContext) {
		ctx.maxParallel = i
	}
}

// WithWriter returns an option to set the writer.
func WithWriter(w io.Writer) Option {
	return func(ctx *testContext) {
		ctx.w = &mutexWriter{w: w}
	}
}

// WithVerboseLog returns an option to enable verbose log.
func WithVerboseLog() Option {
	return func(ctx *testContext) {
		ctx.verbose = true
	}
}

// WithNoColor returns an option to disable colored log.
func WithNoColor() Option {
	return func(ctx *testContext) {
		ctx.noColor = true
	}
}

// WithTestSummary returns an option to enable test summary.
func WithTestSummary() Option {
	return func(ctx *testContext) {
		ctx.enabledTestSummary = true
		ctx.testSummary = newTestSummary()
	}
}

// testContext holds all fields that are common to all tests.
type testContext struct {
	m sync.Mutex

	w io.Writer

	// Channel used to signal tests that are ready to be run in parallel.
	startParallel chan bool

	// running is the number of tests currently running in parallel.
	// This does not include tests that are waiting for subtests to complete.
	running int

	// numWaiting is the number tests waiting to be run in parallel.
	numWaiting int64

	// maxParallel is a copy of the parallel flag.
	maxParallel int

	// verbose indicates that prints verbose log or not.
	verbose bool

	noColor bool

	enabledTestSummary bool
	testSummary        testSummary

	// for FromT
	matcher *matcher
}

func newTestContext(opts ...Option) *testContext {
	ctx := &testContext{
		w:             &nopWriter{},
		startParallel: make(chan bool),
		maxParallel:   1,
		running:       1, // Set the count to 1 for the main (sequential) test.
	}
	for _, opt := range opts {
		opt(ctx)
	}
	return ctx
}

func (c *testContext) waitParallel() {
	c.m.Lock()
	if c.running < c.maxParallel {
		c.running++
		c.m.Unlock()
		return
	}
	atomic.AddInt64(&c.numWaiting, 1)
	c.m.Unlock()
	<-c.startParallel
}

func (c *testContext) waitings() int64 {
	return atomic.LoadInt64(&c.numWaiting)
}

func (c *testContext) release() {
	c.m.Lock()
	if c.waitings() == 0 {
		c.running--
		c.m.Unlock()
		return
	}
	atomic.AddInt64(&c.numWaiting, -1)
	c.m.Unlock()
	c.startParallel <- true // Pick a waiting test to be run.
}

func (c *testContext) printf(format string, a ...interface{}) (int, error) {
	if c.w == nil {
		return 0, nil
	}
	return fmt.Fprintf(c.w, format, a...)
}

type nopWriter struct{}

func (w *nopWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

type mutexWriter struct {
	m sync.Mutex
	w io.Writer
}

func (w *mutexWriter) Write(p []byte) (int, error) {
	w.m.Lock()
	n, err := w.w.Write(p)
	w.m.Unlock()
	return n, err
}
