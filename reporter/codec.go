package reporter

import (
	"fmt"
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
	Parent               string                   `json:"parent,omitempty"`
	Children             []string                 `json:"children,omitempty"`
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
func (r *reporter) ToSerializable(reporterMap map[string]*SerializableReporter) string {
	id := reporterID(r)
	if _, exists := reporterMap[id]; exists {
		return id
	}

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
	reporterMap[id] = sr

	// Convert logs
	if r.logs != nil {
		sr.Logs = r.logs.all()
	}

	// Convert duration
	sr.Duration = r.getDuration()

	if r.parent != nil {
		sr.Parent = r.parent.ToSerializable(reporterMap)
	}

	// Convert children
	if len(r.children) > 0 {
		sr.Children = make([]string, 0, len(r.children))
		for _, child := range r.children {
			sr.Children = append(sr.Children, child.ToSerializable(reporterMap))
		}
	}

	// Convert context
	if r.context != nil {
		sr.Context = r.context.ToSerializable()
	}

	return id
}

func (r *reporter) SetFromSerializable(id string, srMap map[string]*SerializableReporter) {
	rMap := make(map[string]*reporter)
	r.setFromSerializable(srMap[id], rMap, srMap)
}

func (r *reporter) resolveReporter(id string, reporterMap map[string]*reporter, serializedReporterMap map[string]*SerializableReporter) *reporter {
	if rep, exists := reporterMap[id]; exists {
		return rep
	}
	rep, exists := serializedReporterMap[id]
	if !exists {
		return nil
	}
	ret := newReporter()
	reporterMap[id] = ret
	ret.setFromSerializable(rep, reporterMap, serializedReporterMap)
	return ret
}

func (r *reporter) setFromSerializable(sr *SerializableReporter, reporterMap map[string]*reporter, serializedReporterMap map[string]*SerializableReporter) {
	r.parent = r.resolveReporter(sr.Parent, reporterMap, serializedReporterMap)

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
		for _, childID := range sr.Children {
			child := r.resolveReporter(childID, reporterMap, serializedReporterMap)
			child.parent = r
			r.children = append(r.children, child)
		}
	}

	// Set context
	if sr.Context != nil {
		r.context = FromSerializableTestContext(sr.Context)
	}
}

func reporterID(r *reporter) string {
	return fmt.Sprintf("%p", r)
}

// FromSerializable creates a new reporter struct from SerializableReporter.
// This function reconstructs a reporter from serialized data received from WASM plugins.
func FromSerializable(id string, reporterMap map[string]*SerializableReporter) *reporter {
	// Create a new reporter
	r := newReporter()
	r.SetFromSerializable(id, reporterMap)
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
