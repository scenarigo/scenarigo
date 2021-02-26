package reporter

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"
)

// FromT creates Reporter from t.
func FromT(t *testing.T) Reporter {
	return fromT(t)
}

func fromT(t *testing.T) *testReporter {
	return &testReporter{
		T:                t,
		durationMeasurer: &durationMeasurer{},
	}
}

// testReporter is a wrapper to provide Reporter interface for *testing.T.
type testReporter struct {
	*testing.T
	durationMeasurer testDurationMeasurer
	mu               sync.Mutex
}

func (r *testReporter) addBufferDirectly(b []byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	t := reflect.ValueOf(r.T).Elem()
	common := t.FieldByName("common")
	if !common.IsValid() {
		return false
	}
	output := common.FieldByName("output")
	if !output.IsValid() {
		return false
	}
	if output.Type().Kind() != reflect.Slice {
		return false
	}
	if output.Type().Elem().Kind() != reflect.Uint8 {
		return false
	}
	rawOutput := (*[]byte)(unsafe.Pointer(output.UnsafeAddr()))
	*rawOutput = append(*rawOutput, b...)
	return true
}

func (r *testReporter) logBuffer(s string) []byte {
	return append([]byte(pad(s, "")), '\n')
}

func (r *testReporter) log(log string) {
	if ok := r.addBufferDirectly(r.logBuffer(log)); !ok {
		r.T.Log(log)
	}
}

func (r *testReporter) fatal(log string) {
	if ok := r.addBufferDirectly(r.logBuffer(log)); !ok {
		r.T.Fatal(log)
	} else {
		r.T.FailNow()
	}
}

// Log formats its arguments using default formatting, analogous to fmt.Print,
// and records the text in the log.
// The text will be printed only if the test fails or the --verbose flag is set.
func (r *testReporter) Log(args ...interface{}) {
	r.log(fmt.Sprint(args...))
}

// Logf formats its arguments according to the format, analogous to fmt.Printf, and
// records the text in the log.
// The text will be printed only if the test fails or the --verbose flag is set.
func (r *testReporter) Logf(format string, args ...interface{}) {
	r.log(fmt.Sprintf(format, args...))
}

// Error is equivalent to Log followed by Fail.
func (r *testReporter) Error(args ...interface{}) {
	r.Fail()
	r.Log(args...)
}

// Errorf is equivalent to Logf followed by Fail.
func (r *testReporter) Errorf(format string, args ...interface{}) {
	r.Fail()
	r.Logf(format, args...)
}

// Fatal is equivalent to Log followed by FailNow.
func (r *testReporter) Fatal(args ...interface{}) {
	r.fatal(fmt.Sprint(args...))
}

// Fatalf is equivalent to Logf followed by FailNow.
func (r *testReporter) Fatalf(format string, args ...interface{}) {
	r.fatal(fmt.Sprintf(format, args...))
}

// Skip is equivalent to Log followed by SkipNow.
func (r *testReporter) Skip(args ...interface{}) {
	r.Log(args...)
	r.SkipNow()
}

// Skipf is equivalent to Logf followed by SkipNow.
func (r *testReporter) Skipf(format string, args ...interface{}) {
	r.Logf(format, args...)
	r.SkipNow()
}

// Parallel signals that this test is to be run in parallel with (and only with)
// other parallel tests.
func (r *testReporter) Parallel() {
	r.durationMeasurer.stop()
	r.T.Parallel()
	r.durationMeasurer.start()
}

// Run runs f as a subtest of r called name.
func (r *testReporter) Run(name string, f func(t Reporter)) bool {
	return r.T.Run(name, func(t *testing.T) {
		child := fromT(t)
		child.durationMeasurer = r.durationMeasurer.spawn()
		defer func() {
			err := recover()
			child.durationMeasurer.stop()
			if err != nil {
				panic(err)
			}
		}()
		child.durationMeasurer.start()
		f(child)
	})
}

func (r *testReporter) getDuration() time.Duration {
	return r.durationMeasurer.getDuration()
}
