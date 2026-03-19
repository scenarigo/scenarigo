package reporter

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

// FromT creates Reporter from t.
func FromT(t *testing.T, opts ...Option) Reporter {
	t.Helper()

	var m *matcher
	defaultOpts := []Option{
		WithWriter(&testingWriter{t: t}),
	}
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.run=") {
			var err error
			m, err = newMatcher(strings.TrimPrefix(arg, "-test.run="))
			if err != nil {
				t.Fatalf("failed to parse -run flag: %s", err)
			}
		}
		if arg == "-test.v=true" {
			defaultOpts = append(defaultOpts, WithVerboseLog())
		}
	}

	r := newReporter()
	r.context = newTestContext(append(defaultOpts, opts...)...)
	r.context.matcher = m
	r.name = t.Name()
	r.goTestName = t.Name()
	r.testing = true

	stop := r.start()
	t.Cleanup(func() {
		go stop()
		<-r.done
		if r.Failed() {
			t.Fail()
		}
	})

	return r
}

// testingWriter is a wrapper to provide Reporter interface for *testing.T.
type testingWriter struct {
	m sync.Mutex
	t *testing.T
}

func (w *testingWriter) Write(b []byte) (int, error) {
	w.m.Lock()
	defer w.m.Unlock()

	t := reflect.ValueOf(w.t).Elem()
	common := t.FieldByName("common")
	if !common.IsValid() {
		return 0, errors.New("failed to write to *testing.T")
	}
	output := common.FieldByName("output")
	if !output.IsValid() {
		return 0, errors.New("failed to write to *testing.T")
	}
	if output.Type().Kind() != reflect.Slice {
		return 0, errors.New("failed to write to *testing.T")
	}
	if output.Type().Elem().Kind() != reflect.Uint8 {
		return 0, errors.New("failed to write to *testing.T")
	}
	rawOutput := (*[]byte)(unsafe.Pointer(output.UnsafeAddr()))
	*rawOutput = append(*rawOutput, b...)
	return len(b), nil
}
