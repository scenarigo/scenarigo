package reporter

import (
	"testing"
)

// FromT creates Reporter from t.
func FromT(t *testing.T) Reporter {
	return &testReporter{t}
}

// testReporter is a wrapper to provide Reporter interface for *testing.T.
type testReporter struct {
	*testing.T
}

// Run runs f as a subtest of r called name.
func (r *testReporter) Run(name string, f func(t Reporter)) bool {
	return r.T.Run(name, func(t *testing.T) {
		f(FromT(t))
	})
}

// IsTesting flag whether instance of testing.T
func (r *testReporter) IsTesting() bool { return true }
