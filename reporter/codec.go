package reporter

import (
	"time"
)

// SerializableReporter represents a serializable version of reporter struct.
// It contains all the necessary reporter data that can be marshaled to JSON
// for communication between host and WASM plugins.
type SerializableReporter struct {
	Name                 string                   `json:"name"`
	GoTestName           string                   `json:"goTestName"`
	Depth                int                      `json:"depth"`
	Failed               bool                     `json:"failed"`
	Skipped              bool                     `json:"skipped"`
	IsParallel           bool                     `json:"isParallel"`
	Logs                 []string                 `json:"logs,omitempty"`
	Duration             time.Duration            `json:"duration"`
	Children             []*SerializableReporter  `json:"children,omitempty"`
	Testing              bool                     `json:"testing"`
	Retryable            bool                     `json:"retryable"`
	NoFailurePropagation bool                     `json:"noFailurePropagation"`
	Context              *SerializableTestContext `json:"context,omitempty"`
}

// SerializableTestContext represents a serializable version of testContext struct.
// It contains test execution context that can be transmitted to WASM plugins.
type SerializableTestContext struct {
	MaxParallel        int   `json:"maxParallel"`
	Verbose            bool  `json:"verbose"`
	NoColor            bool  `json:"noColor"`
	EnabledTestSummary bool  `json:"enabledTestSummary"`
	Running            int   `json:"running"`
	NumWaiting         int64 `json:"numWaiting"`
}

// ToSerializable converts reporter struct to SerializableReporter.
// This method serializes the reporter state for transmission to WASM plugins.
func (r *reporter) ToSerializable() *SerializableReporter {
	sr := &SerializableReporter{
		Name:                 r.name,
		GoTestName:           r.goTestName,
		Depth:                r.depth,
		Failed:               r.Failed(),
		Skipped:              r.Skipped(),
		IsParallel:           r.isParallel,
		Testing:              r.testing,
		Retryable:            r.retryable,
		NoFailurePropagation: r.noFailurePropagation,
	}

	// Convert logs
	if r.logs != nil {
		sr.Logs = r.logs.all()
	}

	// Convert duration
	sr.Duration = r.getDuration()

	// Convert children
	if len(r.children) > 0 {
		sr.Children = make([]*SerializableReporter, 0, len(r.children))
		for _, child := range r.children {
			sr.Children = append(sr.Children, child.ToSerializable())
		}
	}

	// Convert context
	if r.context != nil {
		sr.Context = r.context.ToSerializable()
	}

	return sr
}

// FromSerializable creates a new reporter struct from SerializableReporter.
// This function reconstructs a reporter from serialized data received from WASM plugins.
func FromSerializable(sr *SerializableReporter) *reporter {
	// Create a new reporter
	r := newReporter()
	r.name = sr.Name
	r.goTestName = sr.GoTestName
	r.depth = sr.Depth
	r.isParallel = sr.IsParallel
	r.testing = sr.Testing
	r.retryable = sr.Retryable
	r.noFailurePropagation = sr.NoFailurePropagation

	if sr.Failed {
		r.Fail()
	}
	if sr.Skipped {
		// Directly set the skipped flag without calling SkipNow() to avoid runtime.Goexit()
		r.skipped = 1
	}

	// Set logs
	if len(sr.Logs) > 0 {
		for _, log := range sr.Logs {
			r.logs.log(log)
		}
	}

	// Set children
	if len(sr.Children) > 0 {
		r.children = make([]*reporter, 0, len(sr.Children))
		for _, childSR := range sr.Children {
			child := FromSerializable(childSR)
			child.parent = r
			r.children = append(r.children, child)
		}
	}

	// Set context
	if sr.Context != nil {
		r.context = FromSerializableTestContext(sr.Context)
	}

	return r
}

// ToSerializable converts testContext struct to SerializableTestContext.
// This method serializes the test context for transmission to WASM plugins.
func (c *testContext) ToSerializable() *SerializableTestContext {
	c.m.Lock()
	defer c.m.Unlock()

	return &SerializableTestContext{
		MaxParallel:        c.maxParallel,
		Verbose:            c.verbose,
		NoColor:            c.noColor,
		EnabledTestSummary: c.enabledTestSummary,
		Running:            c.running,
		NumWaiting:         c.numWaiting,
	}
}

// FromSerializableTestContext creates a new testContext struct from SerializableTestContext.
// This function reconstructs a test context from serialized data received from WASM plugins.
func FromSerializableTestContext(sc *SerializableTestContext) *testContext {
	ctx := &testContext{
		w:                  &nopWriter{},
		startParallel:      make(chan bool),
		maxParallel:        sc.MaxParallel,
		running:            sc.Running,
		numWaiting:         sc.NumWaiting,
		verbose:            sc.Verbose,
		noColor:            sc.NoColor,
		enabledTestSummary: sc.EnabledTestSummary,
	}
	if sc.EnabledTestSummary {
		ctx.testSummary = newTestSummary()
	}
	return ctx
}
