package reporter

import (
	"context"
	"fmt"
	"slices"

	"go.opentelemetry.io/otel/trace"

	"github.com/scenarigo/scenarigo/color"
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
	Duration             int64                    `json:"duration"`
	Parent               string                   `json:"parent,omitempty"`
	Children             []string                 `json:"children,omitempty"`
	Testing              bool                     `json:"testing"`
	Retryable            bool                     `json:"retryable"`
	NoFailurePropagation bool                     `json:"noFailurePropagation"`
	Context              *SerializableTestContext `json:"context,omitempty"`

	// Span context of the node span, for the guest to continue the trace.
	// A reporter restored from these fields never starts a span of its own.
	TraceID       string `json:"traceId,omitempty"`
	SpanID        string `json:"spanId,omitempty"`
	TraceFlags    string `json:"traceFlags,omitempty"`
	TraceState    string `json:"traceState,omitempty"`
	RunID         string `json:"runId,omitempty"`
	NodeKind      string `json:"nodeKind,omitempty"`
	ScenarioIndex *int   `json:"scenarioIndex,omitempty"`
	StepIndex     *int   `json:"stepIndex,omitempty"`
}

// SerializableTestContext represents a serializable version of testContext struct.
// It contains test execution context that can be transmitted to WASM plugins.
type SerializableTestContext struct {
	MaxParallel        int   `json:"maxParallel"`
	Verbose            bool  `json:"verbose"`
	ColorEnabled       bool  `json:"colorEnabled"`
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

	// A sibling may call Parallel() or add a child while this tree is serialized.
	r.m.Lock()
	isParallel := r.isParallel
	children := slices.Clone(r.children)
	r.m.Unlock()

	sr := &SerializableReporter{
		Name:                 r.name,
		GoTestName:           r.goTestName,
		Depth:                r.depth,
		Failed:               r.Failed(),
		Skipped:              r.Skipped(),
		IsParallel:           isParallel,
		Testing:              r.testing,
		Retryable:            r.retryable,
		NoFailurePropagation: r.noFailurePropagation,
		RunID:                r.getRunID(),
		NodeKind:             string(r.getNodeKind()),
		ScenarioIndex:        r.getScenarioIndex(),
		StepIndex:            r.getStepIndex(),
	}
	// Only the node's own span is serialized. A retry attempt or a guest-created
	// node finds its span through its parent after restoration, and must not be
	// reported with a span ID of its own.
	if r.span != nil {
		if sc := r.span.SpanContext(); sc.IsValid() {
			sr.TraceID = sc.TraceID().String()
			sr.SpanID = sc.SpanID().String()
			sr.TraceFlags = sc.TraceFlags().String()
			sr.TraceState = sc.TraceState().String()
		}
	}
	reporterMap[id] = sr

	// Convert logs
	if r.logs != nil {
		sr.Logs = r.logs.all()
	}

	// Convert duration
	sr.Duration = int64(r.getDuration())

	if r.parent != nil {
		sr.Parent = r.parent.ToSerializable(reporterMap)
	}

	// Convert children
	if len(children) > 0 {
		sr.Children = make([]string, 0, len(children))
		for _, child := range children {
			sr.Children = append(sr.Children, child.ToSerializable(reporterMap))
		}
	}

	// Convert context
	if r.context != nil {
		sr.Context = r.context.ToSerializable()
	}

	return id
}

// SetFromSerializable restores all reporter state from a serialized reporter.
// This is used to fully reconstruct a reporter from serialized data.
func (r *reporter) SetFromSerializable(id string, srMap map[string]*SerializableReporter) {
	sr, exists := srMap[id]
	if !exists {
		return
	}
	r.setFromSerializable(sr, map[string]*reporter{id: r}, srMap)
}

// SyncFromSerializable syncs logs and state from a serialized reporter to the current reporter.
// This is used to sync changes made by WASM plugins back to the host's reporter.
// Only logs and failed/skipped state are synced; other fields like parent, children,
// and context are not overwritten to preserve the host's reporter hierarchy.
func (r *reporter) SyncFromSerializable(id string, srMap map[string]*SerializableReporter) {
	sr, exists := srMap[id]
	if !exists {
		return
	}
	// Only sync logs - don't overwrite other fields to preserve host's reporter hierarchy
	if len(sr.Logs) > 0 {
		for _, log := range sr.Logs {
			r.logs.log(log)
		}
	}
	// Sync failed/skipped state
	if sr.Failed {
		r.Fail()
	}
	if sr.Skipped {
		r.skipped = 1
	}
}

func (r *reporter) setFromSerializable(sr *SerializableReporter, reporterMap map[string]*reporter, serializedReporterMap map[string]*SerializableReporter) {
	if sr == nil {
		return
	}
	r.parent = r.resolveReporter(sr.Parent, reporterMap, serializedReporterMap)

	r.name = sr.Name
	r.goTestName = sr.GoTestName
	r.depth = sr.Depth
	r.isParallel = sr.IsParallel
	r.testing = sr.Testing
	r.retryable = sr.Retryable
	r.noFailurePropagation = sr.NoFailurePropagation
	r.remote = true
	r.runID = sr.RunID
	r.nodeKind = NodeKind(sr.NodeKind)
	r.scenarioIndex = sr.ScenarioIndex
	r.stepIndex = sr.StepIndex
	r.span = remoteSpan(sr)

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

func FromSerializableWithReporter(r Reporter, id string, reporterMap map[string]*SerializableReporter) (*reporter, error) {
	if r == nil {
		return FromSerializable(id, reporterMap), nil
	}
	dst, dok := r.(*reporter)
	if !dok {
		return nil, fmt.Errorf("failed to get reporter instance")
	}
	src := FromSerializable(id, reporterMap)
	dst.appendChildren(src.children...)
	return dst, nil
}

// ToSerializable converts testContext struct to SerializableTestContext.
// This method serializes the test context for transmission to WASM plugins.
func (c *testContext) ToSerializable() *SerializableTestContext {
	c.m.Lock()
	defer c.m.Unlock()

	return &SerializableTestContext{
		MaxParallel:        c.maxParallel,
		Verbose:            c.verbose,
		ColorEnabled:       c.colorConfig.IsEnabled(),
		EnabledTestSummary: c.enabledTestSummary,
		Running:            c.running,
		NumWaiting:         c.numWaiting,
	}
}

// FromSerializableTestContext creates a new testContext struct from SerializableTestContext.
// This function reconstructs a test context from serialized data received from WASM plugins.
func FromSerializableTestContext(sc *SerializableTestContext) *testContext {
	ctx := &testContext{
		w:             &nopWriter{},
		startParallel: make(chan bool),
		maxParallel:   sc.MaxParallel,
		running:       sc.Running,
		numWaiting:    sc.NumWaiting,
		verbose:       sc.Verbose,
		// Restore ColorConfig with preserved enabled state
		colorConfig: func() *color.Config {
			c := color.New()
			c.SetEnabled(sc.ColorEnabled)
			return c
		}(),
		enabledTestSummary: sc.EnabledTestSummary,
	}
	if sc.EnabledTestSummary {
		ctx.testSummary = newTestSummary()
	}
	return ctx
}

// remoteSpan rebuilds a non-recording span from the serialized span context.
// It returns nil when the serialized node had no valid span.
func remoteSpan(sr *SerializableReporter) trace.Span {
	if sr.TraceID == "" || sr.SpanID == "" {
		return nil
	}
	traceID, err := trace.TraceIDFromHex(sr.TraceID)
	if err != nil {
		return nil
	}
	spanID, err := trace.SpanIDFromHex(sr.SpanID)
	if err != nil {
		return nil
	}
	cfg := trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, Remote: true}
	if sr.TraceFlags != "" {
		var flags trace.TraceFlags
		if _, err := fmt.Sscanf(sr.TraceFlags, "%02x", &flags); err == nil {
			cfg.TraceFlags = flags
		}
	}
	if sr.TraceState != "" {
		if ts, err := trace.ParseTraceState(sr.TraceState); err == nil {
			cfg.TraceState = ts
		}
	}
	sc := trace.NewSpanContext(cfg)
	if !sc.IsValid() {
		return nil
	}
	return trace.SpanFromContext(trace.ContextWithRemoteSpanContext(context.Background(), sc))
}
