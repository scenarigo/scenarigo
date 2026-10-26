package context

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/scenarigo/scenarigo/reporter"
)

func TestContext_ExecutionMetadata(t *testing.T) {
	ctx := New(nil)
	if ctx.RunID() != "" {
		t.Error("RunID must be empty by default")
	}
	if _, ok := ctx.ScenarioIndex(); ok {
		t.Error("ScenarioIndex must be absent by default")
	}
	if _, ok := ctx.StepIndex(); ok {
		t.Error("StepIndex must be absent by default")
	}
	if _, ok := ctx.TelemetryConfig(); ok {
		t.Error("TelemetryConfig must be absent by default")
	}
	if ctx.hasRootScenario() {
		t.Error("the root scenario must be absent by default")
	}

	ctx = ctx.WithRunID("run-1").WithScenarioIndex(3).WithStepIndex(1).WithStepName("step")
	if got := ctx.RunID(); got != "run-1" {
		t.Errorf("RunID = %q", got)
	}
	if i, ok := ctx.ScenarioIndex(); !ok || i != 3 {
		t.Errorf("ScenarioIndex = (%d, %t)", i, ok)
	}
	if i, ok := ctx.StepIndex(); !ok || i != 1 {
		t.Errorf("StepIndex = (%d, %t)", i, ok)
	}
	if got := ctx.StepName(); got != "step" {
		t.Errorf("StepName = %q", got)
	}

	// WithStepName alone does not make a step index appear.
	if i, ok := New(nil).WithStepName("only name").StepIndex(); ok {
		t.Errorf("StepIndex = (%d, %t) with only a name", i, ok)
	}

	reset := ctx.WithoutStep()
	if _, ok := reset.StepIndex(); ok {
		t.Error("WithoutStep must clear the step index")
	}
	if reset.StepName() != "" {
		t.Error("WithoutStep must clear the step name")
	}
	if i, ok := reset.ScenarioIndex(); !ok || i != 3 {
		t.Error("WithoutStep must keep the scenario layer")
	}

	cfg := TelemetryConfig{Traceparent: false, Baggage: true, LegacyHeaders: false, RootDir: "/root"}
	got, ok := ctx.WithTelemetryConfig(cfg).TelemetryConfig()
	if !ok || got != cfg {
		t.Errorf("TelemetryConfig = (%+v, %t)", got, ok)
	}

	// The root scenario layer survives a change of the scenario layer (an include step).
	rooted := ctx.withRootScenario("root.yaml", new(1)).WithScenarioFilepath("inc.yaml").WithScenarioIndex(0)
	if !rooted.hasRootScenario() || rooted.RootScenarioFilepath() != "root.yaml" {
		t.Errorf("RootScenarioFilepath = %q", rooted.RootScenarioFilepath())
	}
	if i, ok := rooted.RootScenarioIndex(); !ok || i != 1 {
		t.Errorf("RootScenarioIndex = (%d, %t)", i, ok)
	}
	if _, ok := ctx.withRootScenario("root.yaml", nil).RootScenarioIndex(); ok {
		t.Error("RootScenarioIndex must be absent when the root has no index")
	}
}

func TestContext_EnsureRootScenario(t *testing.T) {
	cfg := TelemetryConfig{RootDir: "/cfg"}
	outer := New(nil).WithTelemetryConfig(cfg).WithScenarioFilepath("/cfg/outer.yaml").WithScenarioIndex(2).EnsureRootScenario()
	if got := outer.RootScenarioFilepath(); got != "outer.yaml" {
		t.Errorf("RootScenarioFilepath = %q, want the normalized path of the first scenario", got)
	}
	if i, ok := outer.RootScenarioIndex(); !ok || i != 2 {
		t.Errorf("RootScenarioIndex = (%d, %t)", i, ok)
	}

	// An included scenario changes the scenario layer but keeps the root.
	inner := outer.WithScenarioFilepath("/cfg/inner.yaml").WithScenarioIndex(0).EnsureRootScenario()
	if got := inner.RootScenarioFilepath(); got != "outer.yaml" {
		t.Errorf("RootScenarioFilepath = %q after an include", got)
	}
	if i, ok := inner.RootScenarioIndex(); !ok || i != 2 {
		t.Errorf("RootScenarioIndex = (%d, %t) after an include", i, ok)
	}

	// A scenario without an index records a root without an index.
	if _, ok := New(nil).WithScenarioFilepath("a.yaml").EnsureRootScenario().RootScenarioIndex(); ok {
		t.Error("RootScenarioIndex must be absent when the scenario has none")
	}

	// A context without a scenario has nothing to record: a scenario started on it later is the root.
	if New(nil).EnsureRootScenario().hasRootScenario() {
		t.Error("a root must not be recorded when there is no scenario")
	}
}

func TestContext_NormalizedScenarioFilepath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(wd, "testdata")
	tests := map[string]struct {
		ctx    *Context
		expect string
	}{
		"no config: relative to the working directory": {
			ctx:    New(nil).WithScenarioFilepath(filepath.Join(wd, "a", "b.yaml")),
			expect: "a/b.yaml",
		},
		"config root": {
			ctx:    New(nil).WithTelemetryConfig(TelemetryConfig{RootDir: root}).WithScenarioFilepath(filepath.Join(root, "x", "y.yaml")),
			expect: "x/y.yaml",
		},
		"outside the root uses ..": {
			ctx:    New(nil).WithTelemetryConfig(TelemetryConfig{RootDir: root}).WithScenarioFilepath(filepath.Join(wd, "z.yaml")),
			expect: "../z.yaml",
		},
		"relative input is resolved against the working directory": {
			ctx:    New(nil).WithTelemetryConfig(TelemetryConfig{RootDir: root}).WithScenarioFilepath(filepath.Join("testdata", "r.yaml")),
			expect: "r.yaml",
		},
		"empty": {
			ctx:    New(nil),
			expect: "",
		},
		"pinned value wins": {
			ctx:    New(nil).WithScenarioFilepath("/elsewhere/a.yaml").withNormalizedFilepath("pinned/a.yaml"),
			expect: "pinned/a.yaml",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.ctx.NormalizedScenarioFilepath(); got != test.expect {
				t.Errorf("got %q, want %q", got, test.expect)
			}
		})
	}
}

func recordingReporter(t *testing.T) (reporter.Reporter, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return reporter.FromT(t, reporter.WithTracerProvider(tp)), rec
}

func TestContext_Inject(t *testing.T) {
	r, _ := recordingReporter(t)
	nodeSC := r.Span().SpanContext()
	if !nodeSC.IsValid() {
		t.Fatal("node span must be valid")
	}

	base := New(r).WithRunID("run-1").WithScenarioFilepath("s.yaml").WithScenarioTitle("title").WithScenarioIndex(0).WithStepIndex(2).WithStepName("step").
		withRootScenario("root.yaml", new(1))

	t.Run("defaults write traceparent of the node span and baggage", func(t *testing.T) {
		header := http.Header{}
		base.Inject(propagation.HeaderCarrier(header))
		tp := header.Get("Traceparent")
		if !strings.Contains(tp, nodeSC.TraceID().String()) || !strings.Contains(tp, nodeSC.SpanID().String()) {
			t.Errorf("traceparent %q must name the node span", tp)
		}
		b, err := baggage.Parse(header.Get("Baggage"))
		if err != nil {
			t.Fatal(err)
		}
		expect := map[string]string{
			"scenarigo.run_id": "run-1", "scenarigo.filepath": "s.yaml", "scenarigo.scenario_index": "0",
			"scenarigo.step_index": "2", "scenarigo.scenario_title": "title", "scenarigo.step_name": "step",
			"scenarigo.root_filepath": "root.yaml", "scenarigo.root_scenario_index": "1",
		}
		for k, v := range expect {
			if got := b.Member(k).Value(); got != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}
	})

	t.Run("without a recorded root the scenario is its own root", func(t *testing.T) {
		header := http.Header{}
		New(r).WithScenarioFilepath("s.yaml").WithScenarioIndex(3).Inject(propagation.HeaderCarrier(header))
		b, err := baggage.Parse(header.Get("Baggage"))
		if err != nil {
			t.Fatal(err)
		}
		if got := b.Member("scenarigo.root_filepath").Value(); got != "s.yaml" {
			t.Errorf("root_filepath = %q", got)
		}
		if got := b.Member("scenarigo.root_scenario_index").Value(); got != "3" {
			t.Errorf("root_scenario_index = %q", got)
		}
	})

	t.Run("request context span wins over the node span", func(t *testing.T) {
		reqCtx, span := trace.SpanFromContext(base.RequestContext()).TracerProvider().Tracer("t").Start(base.RequestContext(), "request")
		defer span.End()
		header := http.Header{}
		base.WithRequestContext(reqCtx).Inject(propagation.HeaderCarrier(header))
		if tp := header.Get("Traceparent"); !strings.Contains(tp, span.SpanContext().SpanID().String()) {
			t.Errorf("traceparent %q must name the request span", tp)
		}
	})

	t.Run("config turns headers off", func(t *testing.T) {
		header := http.Header{"Traceparent": {"user"}, "Baggage": {"user=1"}}
		base.WithTelemetryConfig(TelemetryConfig{}).Inject(propagation.HeaderCarrier(header))
		if got := header.Get("Traceparent"); got != "user" {
			t.Errorf("traceparent = %q, want the user's", got)
		}
		if got := header.Get("Baggage"); got != "user=1" {
			t.Errorf("baggage = %q, want the user's", got)
		}
	})

	t.Run("map carrier", func(t *testing.T) {
		carrier := propagation.MapCarrier{"tracestate": "vendor=x"}
		base.Inject(carrier)
		if carrier.Get("traceparent") == "" || carrier.Get("baggage") == "" {
			t.Errorf("headers missing: %v", carrier)
		}
		if _, ok := carrier["tracestate"]; ok {
			t.Error("user tracestate must be removed with the replaced traceparent")
		}
	})

	t.Run("unknown carrier gets the shim", func(t *testing.T) {
		carrier := &sliceCarrier{}
		base.Inject(carrier)
		if carrier.Get("traceparent") == "" || carrier.Get("baggage") == "" {
			t.Errorf("headers missing: %v", carrier.kv)
		}
	})

	t.Run("no reporter: no traceparent, baggage still written", func(t *testing.T) {
		header := http.Header{}
		New(nil).WithRunID("run-1").Inject(propagation.HeaderCarrier(header))
		if got := header.Get("Traceparent"); got != "" {
			t.Errorf("traceparent = %q, want none", got)
		}
		if got := header.Get("Baggage"); got != "scenarigo.run_id=run-1" {
			t.Errorf("baggage = %q", got)
		}
	})
}

type sliceCarrier struct{ kv [][2]string }

func (c *sliceCarrier) Get(key string) string {
	for _, p := range c.kv {
		if p[0] == key {
			return p[1]
		}
	}
	return ""
}

func (c *sliceCarrier) Set(key, value string) {
	for i, p := range c.kv {
		if p[0] == key {
			c.kv[i][1] = value
			return
		}
	}
	c.kv = append(c.kv, [2]string{key, value})
}

func (c *sliceCarrier) Keys() []string {
	keys := make([]string, 0, len(c.kv))
	for _, p := range c.kv {
		keys = append(keys, p[0])
	}
	return keys
}

func TestContext_Tracer(t *testing.T) {
	t.Run("spans go under the node span", func(t *testing.T) {
		r, rec := recordingReporter(t)
		ctx := New(r)
		_, span := ctx.Tracer("plugin").Start(ctx.RequestContext(), "custom")
		span.End()
		spans := rec.Ended()
		if len(spans) != 1 {
			t.Fatalf("expected 1 span, got %d", len(spans))
		}
		if spans[0].Parent().SpanID() != r.Span().SpanContext().SpanID() {
			t.Error("custom span must be a child of the node span")
		}
		if spans[0].InstrumentationScope().Name != "plugin" {
			t.Errorf("scope = %q", spans[0].InstrumentationScope().Name)
		}
	})
	t.Run("no reporter", func(t *testing.T) {
		ctx := New(nil)
		_, span := ctx.Tracer("plugin").Start(context.Background(), "custom")
		if span.SpanContext().IsValid() {
			t.Error("tracer must be a no-op without a reporter")
		}
	})
}

func TestContext_WithReporter_RequestContextCarriesSpan(t *testing.T) {
	r, _ := recordingReporter(t)
	ctx := New(nil).WithReporter(r)
	if got := trace.SpanContextFromContext(ctx.RequestContext()); !got.Equal(r.Span().SpanContext()) {
		t.Errorf("request context span = %v, want the node span", got)
	}
	var child reporter.Reporter
	r.Run("child", func(cr reporter.Reporter) { child = cr })
	if got := trace.SpanContextFromContext(ctx.WithReporter(child).RequestContext()); !got.Equal(child.Span().SpanContext()) {
		t.Error("WithReporter must replace the span with the new node's")
	}
}

func TestContext_Serializable_ExecutionMetadata(t *testing.T) {
	r, _ := recordingReporter(t)
	cfg := TelemetryConfig{Traceparent: true, Baggage: false, LegacyHeaders: true, RootDir: "/cfg"}
	ctx := New(r).WithRunID("run-1").WithScenarioFilepath("/cfg/dir/s.yaml").WithScenarioTitle("title").
		WithScenarioIndex(4).WithStepIndex(1).WithStepName("step").WithTelemetryConfig(cfg).
		withRootScenario("root/r.yaml", new(2))

	sc := ctx.ToSerializable()
	if sc.NormalizedFilepath != "dir/s.yaml" {
		t.Errorf("NormalizedFilepath = %q", sc.NormalizedFilepath)
	}
	b, err := json.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	var decoded SerializableContext
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	// The guest must not recompute the path: give it a root that would yield another value.
	decoded.Telemetry.RootDir = "/elsewhere"
	cfg.RootDir = "/elsewhere"
	restored, err := FromSerializable(&decoded)
	if err != nil {
		t.Fatal(err)
	}
	if restored.RunID() != "run-1" || restored.ScenarioTitle() != "title" || restored.StepName() != "step" {
		t.Errorf("scalar metadata mismatch: %q %q %q", restored.RunID(), restored.ScenarioTitle(), restored.StepName())
	}
	if i, ok := restored.ScenarioIndex(); !ok || i != 4 {
		t.Errorf("ScenarioIndex = (%d, %t)", i, ok)
	}
	if i, ok := restored.StepIndex(); !ok || i != 1 {
		t.Errorf("StepIndex = (%d, %t)", i, ok)
	}
	if got, ok := restored.TelemetryConfig(); !ok || got != cfg {
		t.Errorf("TelemetryConfig = (%+v, %t)", got, ok)
	}
	if got := restored.NormalizedScenarioFilepath(); got != "dir/s.yaml" {
		t.Errorf("NormalizedScenarioFilepath = %q (must not be recomputed on the guest)", got)
	}
	if got := restored.RootScenarioFilepath(); got != "root/r.yaml" {
		t.Errorf("RootScenarioFilepath = %q", got)
	}
	if i, ok := restored.RootScenarioIndex(); !ok || i != 2 {
		t.Errorf("RootScenarioIndex = (%d, %t)", i, ok)
	}
	if got := trace.SpanContextFromContext(restored.RequestContext()); got.SpanID() != r.Span().SpanContext().SpanID() || !got.IsRemote() {
		t.Errorf("restored request context must carry the remote node span, got %v", got)
	}
	_, span := restored.Tracer("x").Start(context.Background(), "guest")
	if span.IsRecording() {
		t.Error("guest-side tracer must not record")
	}

	// Absent metadata stays absent.
	empty, err := FromSerializable(New(nil).ToSerializable())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := empty.ScenarioIndex(); ok {
		t.Error("ScenarioIndex must stay absent")
	}
	if _, ok := empty.StepIndex(); ok {
		t.Error("StepIndex must stay absent")
	}
	if _, ok := empty.TelemetryConfig(); ok {
		t.Error("TelemetryConfig must stay absent")
	}
	if empty.hasRootScenario() {
		t.Error("the root scenario must stay absent")
	}
}

func TestContext_RunID_FromReporter(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	reporter.Run(func(r reporter.Reporter) {
		want := reporter.RunID(r)
		if want == "" {
			t.Fatal("the reporter must have a run ID")
		}
		if got := New(r).RunID(); got != want {
			t.Errorf("RunID() = %q, want the reporter's %q", got, want)
		}
		if got := New(r).WithRunID("host").RunID(); got != "host" {
			t.Errorf("WithRunID must win, got %q", got)
		}
	}, reporter.WithTracerProvider(tp))
	if got := New(nil).RunID(); got != "" {
		t.Errorf("RunID() without a reporter = %q", got)
	}
}

func TestContext_LegacyHeadersEnabled(t *testing.T) {
	tests := map[string]struct {
		ctx  *Context
		want bool
	}{
		"no configuration": {ctx: New(nil), want: true},
		"disabled":         {ctx: New(nil).WithTelemetryConfig(TelemetryConfig{Traceparent: true, Baggage: true}), want: false},
		"enabled":          {ctx: New(nil).WithTelemetryConfig(TelemetryConfig{LegacyHeaders: true}), want: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.ctx.LegacyHeadersEnabled(); got != test.want {
				t.Errorf("LegacyHeadersEnabled() = %t, want %t", got, test.want)
			}
		})
	}
}

// valuesCarrier is a carrier that cannot delete but exposes every value of a key.
type valuesCarrier map[string][]string

func (c valuesCarrier) Get(key string) string {
	if v := c[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}
func (c valuesCarrier) Set(key, value string)      { c[key] = []string{value} }
func (c valuesCarrier) Values(key string) []string { return c[key] }
func (c valuesCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func TestContext_Inject_CarrierWithoutDelete(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	reporter.Run(func(r reporter.Reporter) {
		carrier := valuesCarrier{
			"traceparent": {"00-11111111111111111111111111111111-2222222222222222-01"},
			"tracestate":  {"vendor=x"},
			"baggage":     {"a=1", "b=2"},
		}
		New(r).WithRunID("run-1").Inject(carrier)
		if got := carrier.Get("tracestate"); got != "" {
			t.Errorf("the user tracestate must go with its traceparent, got %q", got)
		}
		if got := carrier.Get("traceparent"); !strings.Contains(got, r.Span().SpanContext().SpanID().String()) {
			t.Errorf("traceparent = %q must name the node span", got)
		}
		bag, err := baggage.Parse(carrier.Get("baggage"))
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range map[string]string{"a": "1", "b": "2", "scenarigo.run_id": "run-1"} {
			if bag.Member(k).Value() != v {
				t.Errorf("baggage %s = %q, want %q (every value of the key must be read)", k, bag.Member(k).Value(), v)
			}
		}
	}, reporter.WithTracerProvider(tp))
}
