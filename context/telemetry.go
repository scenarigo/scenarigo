package context

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/scenarigo/scenarigo/internal/protocolmeta"
	"github.com/scenarigo/scenarigo/reporter"
)

type (
	keyRunID              struct{}
	keyScenarioIndex      struct{}
	keyStepMeta           struct{}
	keyTelemetryConfig    struct{}
	keyNormalizedFilepath struct{}
	keyRootScenario       struct{}
)

// rootScenario names the top-level scenario, the root of the trace. An include
// step replaces the scenario layer of the context but never this one.
type rootScenario struct {
	filepath string // normalized like NormalizedScenarioFilepath
	index    *int
}

// stepMeta is the step layer of the execution metadata. It is replaced as a
// whole so that a step never inherits the name or index of the previous one.
type stepMeta struct {
	index   int
	name    string
	present bool
}

// TelemetryConfig is the part of the configuration that decides what scenarigo
// writes on the wire. It mirrors the "telemetry.propagation" section of the
// configuration file; the context package cannot import schema.
type TelemetryConfig struct {
	Traceparent   bool `json:"traceparent"`
	Baggage       bool `json:"baggage"`
	LegacyHeaders bool `json:"legacyHeaders"`
	// RootDir is the directory of the configuration file. Filepaths in baggage
	// and span attributes are relative to it.
	RootDir string `json:"rootDir,omitempty"`
}

// DefaultTelemetryConfig is what applies when no configuration was loaded:
// every header is written and filepaths are relative to the working directory.
func DefaultTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{Traceparent: true, Baggage: true, LegacyHeaders: true}
}

// WithTelemetryConfig returns a copy of c with the telemetry configuration.
func (c *Context) WithTelemetryConfig(cfg TelemetryConfig) *Context {
	return newContext(context.WithValue(c.ctx, keyTelemetryConfig{}, cfg), c.reqCtx, c.reporter)
}

// TelemetryConfig returns the telemetry configuration and whether one was set.
func (c *Context) TelemetryConfig() (TelemetryConfig, bool) {
	cfg, ok := c.ctx.Value(keyTelemetryConfig{}).(TelemetryConfig)
	return cfg, ok
}

func (c *Context) telemetryConfig() TelemetryConfig {
	if cfg, ok := c.TelemetryConfig(); ok {
		return cfg
	}
	return DefaultTelemetryConfig()
}

// LegacyHeadersEnabled reports whether the Scenarigo-* headers and the
// scenarigo-*-bin metadata that predate the W3C baggage are written
// (telemetry.propagation.legacyHeaders, on when no configuration was loaded).
func (c *Context) LegacyHeadersEnabled() bool {
	return c.telemetryConfig().LegacyHeaders
}

// WithRunID returns a copy of c with the run ID.
func (c *Context) WithRunID(id string) *Context {
	return newContext(context.WithValue(c.ctx, keyRunID{}, id), c.reqCtx, c.reporter)
}

// RunID returns the ID of the scenarigo run, shared by every scenario of one
// invocation: the value set with WithRunID, or else the run ID of the reporter
// tree. It is empty without a reporter whose root has a valid span.
func (c *Context) RunID() string {
	if id, ok := c.ctx.Value(keyRunID{}).(string); ok {
		return id
	}
	return reporter.RunID(c.reporter)
}

// WithScenarioIndex returns a copy of c with the index of the scenario in its file.
func (c *Context) WithScenarioIndex(i int) *Context {
	return newContext(context.WithValue(c.ctx, keyScenarioIndex{}, i), c.reqCtx, c.reporter)
}

// ScenarioIndex returns the zero-based index of the scenario within its YAML
// file, and false outside a scenario loaded from a file.
func (c *Context) ScenarioIndex() (int, bool) {
	i, ok := c.ctx.Value(keyScenarioIndex{}).(int)
	return i, ok
}

// WithStepIndex returns a copy of c with the index of the running step.
func (c *Context) WithStepIndex(i int) *Context {
	m := c.stepMeta()
	m.index = i
	m.present = true
	return newContext(context.WithValue(c.ctx, keyStepMeta{}, m), c.reqCtx, c.reporter)
}

// WithStepName returns a copy of c with the name of the running step.
func (c *Context) WithStepName(name string) *Context {
	m := c.stepMeta()
	m.name = name
	return newContext(context.WithValue(c.ctx, keyStepMeta{}, m), c.reqCtx, c.reporter)
}

// WithoutStep returns a copy of c with no step layer. RunScenario uses it so
// that an included scenario does not report the including step.
func (c *Context) WithoutStep() *Context {
	return newContext(context.WithValue(c.ctx, keyStepMeta{}, stepMeta{}), c.reqCtx, c.reporter)
}

// StepIndex returns the zero-based index of the running step within its
// scenario, and false outside a step.
func (c *Context) StepIndex() (int, bool) {
	m := c.stepMeta()
	return m.index, m.present
}

// StepName returns the title of the running step. It is empty outside a step
// and for a step without a title.
func (c *Context) StepName() string {
	return c.stepMeta().name
}

func (c *Context) stepMeta() stepMeta {
	m, _ := c.ctx.Value(keyStepMeta{}).(stepMeta)
	return m
}

// withNormalizedFilepath pins the filepath value put on the wire. A context
// restored on a WASM guest cannot compute it: the guest has another working directory.
func (c *Context) withNormalizedFilepath(path string) *Context {
	return newContext(context.WithValue(c.ctx, keyNormalizedFilepath{}, path), c.reqCtx, c.reporter)
}

// NormalizedScenarioFilepath returns the scenario filepath relative to the
// configuration directory with "/" separators: the value of the
// "scenarigo.filepath" baggage member and span attribute.
func (c *Context) NormalizedScenarioFilepath() string {
	if p, ok := c.ctx.Value(keyNormalizedFilepath{}).(string); ok {
		return p
	}
	return protocolmeta.NormalizeFilepath(c.telemetryConfig().RootDir, c.ScenarioFilepath())
}

// Tracer returns a tracer from the TracerProvider scenarigo runs with. Pass
// c.RequestContext() to Start so that the span nests under the current node
// span. Without a reporter, or on a WASM guest, the tracer is a no-op.
func (c *Context) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	if c.reporter == nil {
		return trace.SpanFromContext(context.Background()).TracerProvider().Tracer(name, opts...)
	}
	return c.reporter.Span().TracerProvider().Tracer(name, opts...)
}

// Inject writes the W3C traceparent and baggage headers that scenarigo would
// put on a request sent from this context into carrier, honouring the
// telemetry configuration. The traceparent names the current node span
// (or the request span when c.RequestContext() carries one). Plugins that
// send requests with their own clients call this on the outgoing headers.
// Warnings are logged to the reporter.
func (c *Context) Inject(carrier propagation.TextMapCarrier) {
	cfg := c.telemetryConfig()
	warnings := protocolmeta.Inject(c.RequestContext(), asCarrier(carrier), c.metadata(), protocolmeta.InjectOptions{
		Traceparent: cfg.Traceparent,
		Baggage:     cfg.Baggage,
	})
	c.logTelemetryWarnings(warnings)
}

// InjectRequestHeaders is Inject with an explicit context: ctx carries the
// span that the traceparent names. The protocol packages call it with the
// request span they started.
func (c *Context) InjectRequestHeaders(ctx context.Context, carrier propagation.TextMapCarrier) {
	cfg := c.telemetryConfig()
	warnings := protocolmeta.Inject(ctx, asCarrier(carrier), c.metadata(), protocolmeta.InjectOptions{
		Traceparent: cfg.Traceparent,
		Baggage:     cfg.Baggage,
	})
	c.logTelemetryWarnings(warnings)
}

func (c *Context) logTelemetryWarnings(warnings []string) {
	if c.reporter == nil {
		return
	}
	for _, w := range warnings {
		c.reporter.Logf("WARN: %s", w)
	}
}

func (c *Context) metadata() protocolmeta.Metadata {
	// A context that never went through RunScenario (built by a test or a
	// plugin) is its own root, so the root members are never missing while the
	// scenario members are present.
	root := c.EnsureRootScenario()
	meta := protocolmeta.Metadata{
		RunID:         c.RunID(),
		Filepath:      c.NormalizedScenarioFilepath(),
		ScenarioTitle: c.ScenarioTitle(),
		StepName:      c.StepName(),
		RootFilepath:  root.RootScenarioFilepath(),
	}
	if i, ok := c.ScenarioIndex(); ok {
		meta.ScenarioIndex = &i
	}
	if i, ok := c.StepIndex(); ok {
		meta.StepIndex = &i
	}
	if i, ok := root.RootScenarioIndex(); ok {
		meta.RootScenarioIndex = &i
	}
	return meta
}

// withRootScenario returns a copy of c that names the top-level scenario by its
// normalized filepath and, when known, its index in that file. Only
// EnsureRootScenario and the WASM codec set it; plugins read it.
func (c *Context) withRootScenario(normalizedFilepath string, index *int) *Context {
	root := rootScenario{filepath: normalizedFilepath}
	if index != nil {
		i := *index
		root.index = &i
	}
	return newContext(context.WithValue(c.ctx, keyRootScenario{}, root), c.reqCtx, c.reporter)
}

// EnsureRootScenario records the current scenario as the top-level one unless a
// root is already recorded: the first scenario started on a context is the root,
// and an include step runs the included scenario on a context derived from it.
// A context without a scenario filepath and index has nothing to record.
func (c *Context) EnsureRootScenario() *Context {
	if c.hasRootScenario() {
		return c
	}
	var idx *int
	if i, ok := c.ScenarioIndex(); ok {
		idx = &i
	}
	filepath := c.NormalizedScenarioFilepath()
	if filepath == "" && idx == nil {
		return c
	}
	return c.withRootScenario(filepath, idx)
}

// hasRootScenario reports whether a top-level scenario was recorded on c.
func (c *Context) hasRootScenario() bool {
	_, ok := c.ctx.Value(keyRootScenario{}).(rootScenario)
	return ok
}

// RootScenarioFilepath returns the normalized filepath of the top-level scenario.
// It differs from NormalizedScenarioFilepath inside an included scenario.
func (c *Context) RootScenarioFilepath() string {
	root, _ := c.ctx.Value(keyRootScenario{}).(rootScenario)
	return root.filepath
}

// RootScenarioIndex returns the index of the top-level scenario in its file.
func (c *Context) RootScenarioIndex() (int, bool) {
	root, _ := c.ctx.Value(keyRootScenario{}).(rootScenario)
	if root.index == nil {
		return 0, false
	}
	return *root.index, true
}

// asCarrier upgrades the common carriers to protocolmeta.Carrier
// (http.Header and metadata.MD arrive as protocolmeta.HeaderCarrier and
// protocolmeta.MDCarrier). Any other carrier gets a shim: a deleted key is set
// to the empty value, which receivers treat as absent, and the values of a
// key are all read only when the carrier implements propagation.ValuesGetter.
func asCarrier(carrier propagation.TextMapCarrier) protocolmeta.Carrier {
	switch v := carrier.(type) {
	case protocolmeta.Carrier:
		return v
	case propagation.HeaderCarrier:
		return protocolmeta.HeaderCarrier(v)
	case propagation.MapCarrier:
		return mapCarrier(v)
	default:
		return shimCarrier{TextMapCarrier: carrier}
	}
}

type mapCarrier propagation.MapCarrier

func (c mapCarrier) Get(key string) string      { return propagation.MapCarrier(c).Get(key) }
func (c mapCarrier) Set(key, value string)      { propagation.MapCarrier(c).Set(key, value) }
func (c mapCarrier) Keys() []string             { return propagation.MapCarrier(c).Keys() }
func (c mapCarrier) Values(key string) []string { return valuesFromGet(c, key) }
func (c mapCarrier) Delete(key string)          { delete(c, key) }

type shimCarrier struct{ propagation.TextMapCarrier }

func (c shimCarrier) Values(key string) []string {
	if g, ok := c.TextMapCarrier.(propagation.ValuesGetter); ok {
		return g.Values(key)
	}
	return valuesFromGet(c, key)
}

func (c shimCarrier) Delete(key string) { c.Set(key, "") }

func valuesFromGet(c propagation.TextMapCarrier, key string) []string {
	if v := c.Get(key); v != "" {
		return []string{v}
	}
	return nil
}

// reporterSpanContext puts the node span of r into reqCtx so that request spans
// and user tracers start under it.
func reporterSpanContext(reqCtx context.Context, r reporter.Reporter) context.Context {
	if r == nil {
		return reqCtx
	}
	return trace.ContextWithSpan(reqCtx, r.Span())
}
