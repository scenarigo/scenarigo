package reporter

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/scenarigo/scenarigo/internal/protocolmeta"
)

const tracerName = "github.com/scenarigo/scenarigo/reporter"

// NodeKind classifies a reporter node. It is exported to the trace as the
// "scenarigo.node" attribute.
type NodeKind string

// Node kinds. Nodes created through the public Context.Run API are substeps.
const (
	NodeKindRun      NodeKind = "run"
	NodeKindFile     NodeKind = "file"
	NodeKindScenario NodeKind = "scenario"
	NodeKindStep     NodeKind = "step"
	NodeKindSetup    NodeKind = "setup"
	NodeKindTeardown NodeKind = "teardown"
	NodeKindInclude  NodeKind = "include"
	NodeKindSubstep  NodeKind = "substep"
)

type keyNewTrace struct{}

// WithNewTrace marks ctx so that the node created by the next RunWithRetry call
// starts a new trace instead of joining its parent's. The new root span links to
// the run span. The runner uses it for each top-level scenario.
func WithNewTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, keyNewTrace{}, true)
}

// WithoutNewTrace cancels WithNewTrace for ctx and the contexts derived from it.
// Context values cannot be removed, so the request context handed to the node's
// function must be marked this way or every nested node would start a new trace.
func WithoutNewTrace(ctx context.Context) context.Context {
	if ctx.Value(keyNewTrace{}) == nil {
		return ctx
	}
	return context.WithValue(ctx, keyNewTrace{}, false)
}

func newTraceRequested(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(keyNewTrace{}).(bool)
	return v
}

// WithTracerProvider returns an option to set the TracerProvider used for node spans.
// nil means the default: an SDK provider without processors, owned and shut down by
// the root node. The provider's sampler is left alone, so a non-sampled or no-op
// provider yields spans that are not exported or carry no valid IDs.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(ctx *testContext) {
		ctx.tracerProvider = tp
		ctx.ownsTracerProvider = false
	}
}

func (c *testContext) ensureTracerProvider() {
	if c.tracerProvider != nil {
		return
	}
	c.tracerProvider = sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	c.ownsTracerProvider = true
}

func (c *testContext) shutdownTracerProvider() {
	if !c.ownsTracerProvider {
		return
	}
	if tp, ok := c.tracerProvider.(*sdktrace.TracerProvider); ok {
		_ = tp.Shutdown(context.Background())
	}
}

// SetNodeKind records what kind of node r is. The runner calls it right after
// creating a node; retry attempts forward it to the node they belong to.
func SetNodeKind(r Reporter, kind NodeKind) {
	r.setNodeKind(kind)
}

// SetScenarioIndex records the scenario index for the test report and the trace.
func SetScenarioIndex(r Reporter, i int) {
	r.setScenarioIndex(i)
}

// SetStepIndex records the step index for the test report and the trace.
func SetStepIndex(r Reporter, i int) {
	r.setStepIndex(i)
}

// SetSpanAttributes sets attributes on the node span of r.
func SetSpanAttributes(r Reporter, attrs ...attribute.KeyValue) {
	r.setSpanAttributes(attrs...)
}

// RunID returns the run ID of the tree r belongs to: the trace ID of the run span,
// or the value the host serialized for a reporter restored on the WASM guest side.
// It is empty when the root has no valid span (no-op provider).
func RunID(r Reporter) string {
	if r == nil {
		return ""
	}
	return r.getRunID()
}

func noopSpan() trace.Span {
	return trace.SpanFromContext(context.Background())
}

func (r *reporter) root() *reporter {
	root := r
	for root.parent != nil {
		root = root.parent
	}
	return root
}

// Span returns the node span. Retry attempts share the span of the node they
// retry; nodes restored from a WASM guest return the nearest serialized span.
// Without a span the result is a no-op span with an invalid span context.
func (r *reporter) Span() trace.Span {
	if r.retryable && r.parent != nil {
		return r.parent.Span()
	}
	if r.span != nil {
		return r.span
	}
	if r.remote && r.parent != nil {
		return r.parent.Span()
	}
	return noopSpan()
}

// startSpan starts the node span. Retry attempts, restored (remote) nodes and
// nodes without a provider start nothing.
func (r *reporter) startSpan() {
	if r.retryable || r.remote || r.context == nil || r.context.tracerProvider == nil {
		return
	}
	name := r.name
	parentCtx := context.Background()
	opts := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindInternal)}
	switch {
	case r.parent == nil:
		if name == "" {
			name = string(NodeKindRun)
		}
		if r.nodeKind == "" {
			r.nodeKind = NodeKindRun
		}
	case r.newTrace:
		// parentCtx stays empty: the span becomes a root of a new trace.
		if sc := r.root().Span().SpanContext(); sc.IsValid() {
			opts = append(opts, trace.WithLinks(trace.Link{SpanContext: sc}))
		}
	default:
		parentCtx = trace.ContextWithSpan(parentCtx, r.parent.Span())
	}
	if r.parent != nil && r.nodeKind == "" {
		// Nodes created through the public Run API; the runner overrides this
		// right after the node starts.
		r.nodeKind = NodeKindSubstep
	}
	_, r.span = r.context.tracerProvider.Tracer(tracerName).Start(parentCtx, name, opts...) //nolint:spancheck // stored on the node, ended by endSpan

	var attrs []attribute.KeyValue
	if runID := r.getRunID(); runID != "" {
		attrs = append(attrs, protocolmeta.AttrRunID.String(runID))
	}
	if r.nodeKind != "" {
		attrs = append(attrs, protocolmeta.AttrNode.String(string(r.nodeKind)))
	}
	if r.scenarioIndex != nil {
		attrs = append(attrs, protocolmeta.AttrScenarioIndex.Int(*r.scenarioIndex))
	}
	if r.stepIndex != nil {
		attrs = append(attrs, protocolmeta.AttrStepIndex.Int(*r.stepIndex))
	}
	if len(attrs) > 0 {
		r.span.SetAttributes(attrs...)
	}
}

// endSpan records the node result on the span and ends it. The description of an
// Error status stays empty: error logs carry request and response bodies.
func (r *reporter) endSpan() {
	if r.span == nil || r.retryable || r.remote {
		return
	}
	switch {
	case r.Failed():
		r.span.SetStatus(codes.Error, "")
	case r.Skipped():
		r.span.SetAttributes(protocolmeta.AttrSkipped.Bool(true))
	default:
		r.span.SetStatus(codes.Ok, "")
	}
	r.span.End()
}

func (r *reporter) recordRetry(attempt int) {
	r.Span().AddEvent("retry", trace.WithAttributes(protocolmeta.AttrAttempt.Int(attempt)))
}

func (r *reporter) setNodeKind(kind NodeKind) {
	if r.retryable && r.parent != nil {
		r.parent.setNodeKind(kind)
		return
	}
	r.m.Lock()
	r.nodeKind = kind
	r.m.Unlock()
	if r.span != nil {
		r.span.SetAttributes(protocolmeta.AttrNode.String(string(kind)))
	}
}

func (r *reporter) setScenarioIndex(i int) {
	if r.retryable && r.parent != nil {
		r.parent.setScenarioIndex(i)
		return
	}
	r.m.Lock()
	r.scenarioIndex = &i
	r.m.Unlock()
	if r.span != nil {
		r.span.SetAttributes(protocolmeta.AttrScenarioIndex.Int(i))
	}
}

func (r *reporter) setStepIndex(i int) {
	if r.retryable && r.parent != nil {
		r.parent.setStepIndex(i)
		return
	}
	r.m.Lock()
	r.stepIndex = &i
	r.m.Unlock()
	if r.span != nil {
		r.span.SetAttributes(protocolmeta.AttrStepIndex.Int(i))
	}
}

func (r *reporter) setSpanAttributes(attrs ...attribute.KeyValue) {
	if len(attrs) == 0 {
		return
	}
	r.Span().SetAttributes(attrs...)
}

func (r *reporter) getNodeKind() NodeKind {
	if r.retryable && r.parent != nil {
		return r.parent.getNodeKind()
	}
	r.m.Lock()
	defer r.m.Unlock()
	return r.nodeKind
}

func (r *reporter) getScenarioIndex() *int {
	if r.retryable && r.parent != nil {
		return r.parent.getScenarioIndex()
	}
	r.m.Lock()
	defer r.m.Unlock()
	return r.scenarioIndex
}

func (r *reporter) getStepIndex() *int {
	if r.retryable && r.parent != nil {
		return r.parent.getStepIndex()
	}
	r.m.Lock()
	defer r.m.Unlock()
	return r.stepIndex
}

func (r *reporter) getRunID() string {
	if r.runID != "" {
		return r.runID
	}
	if sc := r.root().Span().SpanContext(); sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}

func (r *reporter) getTraceID() string {
	if sc := r.Span().SpanContext(); sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}

// getSpanID returns the ID of the node's own span. Retry attempts and remote
// descendants share an ancestor's span, which the report must not repeat.
func (r *reporter) getSpanID() string {
	if r.span == nil {
		return ""
	}
	if sc := r.span.SpanContext(); sc.HasSpanID() {
		return sc.SpanID().String()
	}
	return ""
}
