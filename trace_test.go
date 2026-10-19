package scenarigo

import (
	gocontext "context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
)

type capturedRequest struct {
	path   string
	header http.Header
}

func attrOf(s sdktrace.ReadOnlySpan, key attribute.Key) (attribute.Value, bool) {
	for _, kv := range s.Attributes() {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestRunner_Trace(t *testing.T) {
	var mu sync.Mutex
	var requests []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		requests = append(requests, capturedRequest{path: req.URL.Path, header: req.Header.Clone()})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)

	yml := `
title: first
steps:
- title: GET /a
  protocol: http
  request:
    url: "` + srv.URL + `/a"
- title: GET /b
  protocol: http
  request:
    url: "` + srv.URL + `/b"
---
title: second
steps:
- title: GET /c
  protocol: http
  request:
    url: "` + srv.URL + `/c"
`
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(gocontext.Background()) })

	runner, err := NewRunner(
		WithScenariosFromReader(strings.NewReader(yml)),
		WithConfig(&schema.Config{
			Telemetry: schema.TelemetryConfig{Propagation: schema.PropagationConfig{LegacyHeaders: new(false)}},
		}),
	)
	if err != nil {
		t.Fatalf("failed to create runner: %s", err)
	}
	var report *reporter.TestReport
	ok := reporter.Run(func(rptr reporter.Reporter) {
		runner.Run(context.New(rptr))
		rptr.Cleanup(func() {
			report, err = reporter.GenerateTestReport(rptr)
		})
	}, reporter.WithTracerProvider(tp))
	if !ok || err != nil {
		t.Fatalf("run failed: ok=%t err=%v", ok, err)
	}

	spans := rec.Ended()
	byName := map[string]sdktrace.ReadOnlySpan{}
	for _, s := range spans {
		byName[s.Name()] = s
	}
	need := func(name string) sdktrace.ReadOnlySpan {
		s, ok := byName[name]
		if !ok {
			names := make([]string, 0, len(spans))
			for _, s := range spans {
				names = append(names, s.Name())
			}
			t.Fatalf("span %q missing in %v", name, names)
		}
		return s
	}
	run := need("run")
	file := need("0")
	first := need("first")
	second := need("second")
	stepA := need("GET /a")
	stepB := need("GET /b")
	stepC := need("GET /c")
	reqA := need("GET")
	if got := len(spans); got != 10 {
		t.Errorf("expected 10 spans (run, file, 2 scenarios, 3 steps, 3 requests), got %d", got)
	}

	runID := run.SpanContext().TraceID().String()
	kinds := map[string]string{"run": "run", "0": "file", "first": "scenario", "GET /a": "step"}
	for name, kind := range kinds {
		if v, ok := attrOf(need(name), "scenarigo.node"); !ok || v.AsString() != kind {
			t.Errorf("%s: scenarigo.node = %q, want %q", name, v.AsString(), kind)
		}
	}
	if file.Parent().SpanID() != run.SpanContext().SpanID() || file.SpanContext().TraceID() != run.SpanContext().TraceID() {
		t.Error("file node must be a child of the run span in the run trace")
	}
	for _, scn := range []sdktrace.ReadOnlySpan{first, second} {
		if scn.SpanContext().TraceID() == run.SpanContext().TraceID() || scn.Parent().IsValid() {
			t.Errorf("%s must be the root of its own trace", scn.Name())
		}
		if links := scn.Links(); len(links) != 1 || links[0].SpanContext.SpanID() != run.SpanContext().SpanID() {
			t.Errorf("%s must link to the run span", scn.Name())
		}
		if v, ok := attrOf(scn, "scenarigo.run_id"); !ok || v.AsString() != runID {
			t.Errorf("%s: scenarigo.run_id = %q", scn.Name(), v.AsString())
		}
		if v, ok := attrOf(scn, "scenarigo.filepath"); !ok || v.AsString() == "" {
			t.Errorf("%s: scenarigo.filepath missing", scn.Name())
		}
	}
	if first.SpanContext().TraceID() == second.SpanContext().TraceID() {
		t.Error("each scenario must have its own trace")
	}
	if v, ok := attrOf(first, "scenarigo.scenario_index"); !ok || v.AsInt64() != 0 {
		t.Errorf("first: scenario_index = %v", v.AsInt64())
	}
	if v, ok := attrOf(second, "scenarigo.scenario_index"); !ok || v.AsInt64() != 1 {
		t.Errorf("second: scenario_index = %v", v.AsInt64())
	}
	if v, ok := attrOf(first, "scenarigo.scenario_title"); !ok || v.AsString() != "first" {
		t.Errorf("first: scenario_title = %q", v.AsString())
	}
	if stepA.Parent().SpanID() != first.SpanContext().SpanID() || stepB.Parent().SpanID() != first.SpanContext().SpanID() {
		t.Error("steps must be children of their scenario span")
	}
	if v, ok := attrOf(stepB, "scenarigo.step_index"); !ok || v.AsInt64() != 1 {
		t.Errorf("GET /b: step_index = %v", v.AsInt64())
	}
	if v, ok := attrOf(stepB, "scenarigo.step_name"); !ok || v.AsString() != "GET /b" {
		t.Errorf("GET /b: step_name = %q", v.AsString())
	}
	if reqA.SpanKind() != trace.SpanKindClient {
		t.Errorf("request span kind = %v", reqA.SpanKind())
	}

	// Request spans: one per step, each a child of its step span.
	stepByID := map[trace.SpanID]sdktrace.ReadOnlySpan{
		stepA.SpanContext().SpanID(): stepA,
		stepB.SpanContext().SpanID(): stepB,
		stepC.SpanContext().SpanID(): stepC,
	}
	var requestSpans int
	for _, s := range spans {
		if s.SpanKind() != trace.SpanKindClient {
			continue
		}
		requestSpans++
		if _, ok := stepByID[s.Parent().SpanID()]; !ok {
			t.Errorf("request span %s has an unexpected parent", s.Name())
		}
	}
	if requestSpans != 3 {
		t.Errorf("expected 3 request spans, got %d", requestSpans)
	}

	// On the wire: traceparent names a request span, baggage carries the indices.
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(requests))
	}
	stepOf := map[string]sdktrace.ReadOnlySpan{"/a": stepA, "/b": stepB, "/c": stepC}
	for _, req := range requests {
		tp := req.header.Get("Traceparent")
		var named sdktrace.ReadOnlySpan
		for _, s := range spans {
			if s.SpanKind() == trace.SpanKindClient && strings.Contains(tp, s.SpanContext().SpanID().String()) {
				named = s
			}
		}
		step := stepOf[req.path]
		switch {
		case named == nil:
			t.Errorf("%s: traceparent %q names no request span", req.path, tp)
		case named.Parent().SpanID() != step.SpanContext().SpanID():
			t.Errorf("%s: the request span named by traceparent is not a child of step %q", req.path, step.Name())
		case named.SpanContext().TraceID() != step.SpanContext().TraceID():
			t.Errorf("%s: traceparent %q is not in the trace of step %q", req.path, tp, step.Name())
		}
		bag, err := baggage.Parse(req.header.Get("Baggage"))
		if err != nil {
			t.Fatalf("%s: baggage %q: %s", req.path, req.header.Get("Baggage"), err)
		}
		if bag.Member("scenarigo.run_id").Value() != runID {
			t.Errorf("%s: baggage run_id = %q", req.path, bag.Member("scenarigo.run_id").Value())
		}
		if req.header.Get("Scenarigo-Scenario-Filepath") != "" {
			t.Errorf("%s: legacy headers must be off", req.path)
		}
		switch req.path {
		case "/a":
			if bag.Member("scenarigo.scenario_index").Value() != "0" || bag.Member("scenarigo.step_index").Value() != "0" {
				t.Errorf("/a: baggage %s", req.header.Get("Baggage"))
			}
		case "/b":
			if bag.Member("scenarigo.step_index").Value() != "1" || bag.Member("scenarigo.step_name").Value() != "GET /b" {
				t.Errorf("/b: baggage %s", req.header.Get("Baggage"))
			}
		case "/c":
			if bag.Member("scenarigo.scenario_index").Value() != "1" || bag.Member("scenarigo.scenario_title").Value() != "second" {
				t.Errorf("/c: baggage %s", req.header.Get("Baggage"))
			}
		}
	}

	// Report: IDs come from the same spans.
	if report.RunID != runID {
		t.Errorf("report runId = %q", report.RunID)
	}
	scn := report.Files[0].Scenarios[1]
	if scn.TraceID != second.SpanContext().TraceID().String() || scn.ScenarioIndex == nil || *scn.ScenarioIndex != 1 {
		t.Errorf("report scenario: %+v", scn)
	}
	if st := scn.Steps[0]; st.SpanID != stepC.SpanContext().SpanID().String() || st.StepIndex == nil || *st.StepIndex != 0 {
		t.Errorf("report step: %+v", st)
	}

	// The JSON keys are the documented ones.
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"runId":"` + runID + `"`,
		`"traceId":"` + second.SpanContext().TraceID().String() + `"`,
		`"spanId":"` + stepC.SpanContext().SpanID().String() + `"`,
		`"scenarioIndex":1`,
		`"stepIndex":0`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON report lacks %s", want)
		}
	}
}

func TestRunner_Trace_IncludeKeepsTheRootScenario(t *testing.T) {
	var mu sync.Mutex
	var requests []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		requests = append(requests, capturedRequest{path: req.URL.Path, header: req.Header.Clone()})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	root := write("root.yaml", `
title: plain
steps:
- title: GET /x
  protocol: http
  request:
    url: "`+srv.URL+`/x"
---
title: with include
steps:
- title: include inc.yaml
  include: inc.yaml
`)
	write("inc.yaml", `
title: included
steps:
- title: GET /d
  protocol: http
  request:
    url: "`+srv.URL+`/d"
`)

	// WithConfig replaces the scenario list, so the file goes through the config.
	runner, err := NewRunner(WithConfig(&schema.Config{
		Root:      dir,
		Scenarios: []string{filepath.Base(root)},
		Telemetry: schema.TelemetryConfig{Propagation: schema.PropagationConfig{LegacyHeaders: new(false)}},
	}))
	if err != nil {
		t.Fatalf("failed to create runner: %s", err)
	}
	ok := reporter.Run(func(rptr reporter.Reporter) {
		runner.Run(context.New(rptr))
	})
	if !ok {
		t.Fatal("run failed")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	for _, req := range requests {
		bag, err := baggage.Parse(req.header.Get("Baggage"))
		if err != nil {
			t.Fatalf("%s: baggage %q: %s", req.path, req.header.Get("Baggage"), err)
		}
		member := func(key string) string { return bag.Member(key).Value() }
		switch req.path {
		case "/x":
			// A top-level scenario is its own root.
			if member("scenarigo.filepath") != "root.yaml" || member("scenarigo.scenario_index") != "0" {
				t.Errorf("/x: scenario members: %s", req.header.Get("Baggage"))
			}
			if member("scenarigo.root_filepath") != "root.yaml" || member("scenarigo.root_scenario_index") != "0" {
				t.Errorf("/x: root members: %s", req.header.Get("Baggage"))
			}
		case "/d":
			// Inside the include the scenario members describe inc.yaml; the root members still name root.yaml.
			if member("scenarigo.filepath") != "inc.yaml" || member("scenarigo.scenario_index") != "0" || member("scenarigo.scenario_title") != "included" {
				t.Errorf("/d: scenario members: %s", req.header.Get("Baggage"))
			}
			if member("scenarigo.root_filepath") != "root.yaml" || member("scenarigo.root_scenario_index") != "1" {
				t.Errorf("/d: root members: %s", req.header.Get("Baggage"))
			}
		default:
			t.Errorf("unexpected request %s", req.path)
		}
	}
}
