package reporter

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newRecordingProvider(t *testing.T) (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec), sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp, rec
}

func spanByName(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range spans {
		if s.Name() == name {
			return s
		}
	}
	t.Fatalf("span %q not found", name)
	return nil
}

func attrValue(s sdktrace.ReadOnlySpan, key attribute.Key) (attribute.Value, bool) {
	for _, kv := range s.Attributes() {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestNodeSpans(t *testing.T) {
	tp, rec := newRecordingProvider(t)
	var runID string
	Run(func(r Reporter) {
		runID = RunID(r)
		r.Run("pass", func(r Reporter) {
			SetNodeKind(r, NodeKindScenario)
			SetScenarioIndex(r, 2)
			r.Run("fail", func(r Reporter) {
				SetNodeKind(r, NodeKindStep)
				SetStepIndex(r, 0)
				r.Error("boom")
			})
			r.Run("skip", func(r Reporter) {
				r.Skip("skipped")
			})
		})
	}, WithTracerProvider(tp))

	spans := rec.Ended()
	if got, expect := len(spans), 4; got != expect {
		t.Fatalf("expected %d spans but got %d", expect, got)
	}
	root := spanByName(t, spans, "run")
	pass := spanByName(t, spans, "pass")
	fail := spanByName(t, spans, "fail")
	skip := spanByName(t, spans, "skip")

	if runID == "" || runID != root.SpanContext().TraceID().String() {
		t.Errorf("run ID %q must be the trace ID of the run span %q", runID, root.SpanContext().TraceID())
	}
	if !root.Parent().IsValid() == false {
		t.Errorf("run span must be a root span")
	}
	for _, tc := range []struct {
		name   string
		span   sdktrace.ReadOnlySpan
		parent sdktrace.ReadOnlySpan
	}{
		{"pass", pass, root},
		{"fail", fail, pass},
		{"skip", skip, pass},
	} {
		if tc.span.Parent().SpanID() != tc.parent.SpanContext().SpanID() {
			t.Errorf("%s: parent span mismatch", tc.name)
		}
		if tc.span.SpanContext().TraceID() != root.SpanContext().TraceID() {
			t.Errorf("%s: must share the trace of the run span", tc.name)
		}
		if v, ok := attrValue(tc.span, "scenarigo.run_id"); !ok || v.AsString() != runID {
			t.Errorf("%s: scenarigo.run_id = %v, want %s", tc.name, v.AsString(), runID)
		}
	}

	if v, ok := attrValue(root, "scenarigo.node"); !ok || v.AsString() != "run" {
		t.Errorf("run span scenarigo.node = %v", v.AsString())
	}
	if v, ok := attrValue(pass, "scenarigo.node"); !ok || v.AsString() != "scenario" {
		t.Errorf("pass span scenarigo.node = %v", v.AsString())
	}
	if v, ok := attrValue(pass, "scenarigo.scenario_index"); !ok || v.AsInt64() != 2 {
		t.Errorf("pass span scenarigo.scenario_index = %v", v.AsInt64())
	}
	if v, ok := attrValue(fail, "scenarigo.step_index"); !ok || v.AsInt64() != 0 {
		t.Errorf("fail span scenarigo.step_index = %v, present=%t", v.AsInt64(), ok)
	}
	if _, ok := attrValue(pass, "scenarigo.step_index"); ok {
		t.Errorf("pass span must not have scenarigo.step_index")
	}

	if root.Status().Code != codes.Error {
		t.Errorf("run span status = %v, want Error (failure propagates)", root.Status().Code)
	}
	if fail.Status().Code != codes.Error {
		t.Errorf("fail span status = %v, want Error", fail.Status().Code)
	}
	if fail.Status().Description != "" {
		t.Errorf("fail span description must stay empty, got %q", fail.Status().Description)
	}
	if skip.Status().Code != codes.Unset {
		t.Errorf("skip span status = %v, want Unset", skip.Status().Code)
	}
	if v, ok := attrValue(skip, "scenarigo.skipped"); !ok || !v.AsBool() {
		t.Errorf("skip span must have scenarigo.skipped=true")
	}
	if _, ok := attrValue(fail, "scenarigo.skipped"); ok {
		t.Errorf("fail span must not have scenarigo.skipped")
	}
}

func TestNodeSpans_DefaultProvider(t *testing.T) {
	var runID string
	var childSpan trace.Span
	Run(func(r Reporter) {
		runID = RunID(r)
		r.Run("child", func(r Reporter) {
			childSpan = r.Span()
		})
	})
	if runID == "" {
		t.Fatal("run ID must be set with the default provider")
	}
	if !childSpan.SpanContext().IsValid() {
		t.Fatal("child span must carry valid IDs with the default provider")
	}
	if childSpan.SpanContext().TraceID().String() != runID {
		t.Errorf("child trace ID %s != run ID %s", childSpan.SpanContext().TraceID(), runID)
	}
}

func TestNodeSpans_Retry(t *testing.T) {
	tp, rec := newRecordingProvider(t)
	policy := &constantRetryPolicy{interval: time.Millisecond, maxRetries: 2}
	var attemptSpans []trace.Span
	Run(func(r Reporter) {
		var i int
		RunWithRetry(context.Background(), r, "flaky", func(r Reporter) {
			attemptSpans = append(attemptSpans, r.Span())
			SetStepIndex(r, 3)
			i++
			if i < 3 {
				r.Fatal("fail")
			}
		}, policy)
	}, WithTracerProvider(tp))

	spans := rec.Ended()
	if got, expect := len(spans), 2; got != expect {
		t.Fatalf("expected %d spans (run, flaky) but got %d", expect, got)
	}
	flaky := spanByName(t, spans, "flaky")
	if got, expect := len(attemptSpans), 3; got != expect {
		t.Fatalf("expected %d attempts but got %d", expect, got)
	}
	for i, s := range attemptSpans {
		if s.SpanContext().SpanID() != flaky.SpanContext().SpanID() {
			t.Errorf("attempt %d must share the node span", i)
		}
	}
	events := flaky.Events()
	if got, expect := len(events), 2; got != expect {
		t.Fatalf("expected %d retry events but got %d", expect, got)
	}
	for i, ev := range events {
		if ev.Name != "retry" {
			t.Errorf("event %d name = %q", i, ev.Name)
		}
		var attempt int64 = -1
		for _, kv := range ev.Attributes {
			if kv.Key == "scenarigo.attempt" {
				attempt = kv.Value.AsInt64()
			}
		}
		if attempt != int64(i+1) {
			t.Errorf("event %d attempt = %d, want %d", i, attempt, i+1)
		}
	}
	if v, ok := attrValue(flaky, "scenarigo.step_index"); !ok || v.AsInt64() != 3 {
		t.Errorf("step index set on a retry attempt must land on the node span, got %v (present=%t)", v.AsInt64(), ok)
	}
	if flaky.Status().Code != codes.Ok {
		t.Errorf("flaky span status = %v, want Ok after a successful attempt", flaky.Status().Code)
	}
}

func TestNodeSpans_NewTrace(t *testing.T) {
	tp, rec := newRecordingProvider(t)
	var runID string
	Run(func(r Reporter) {
		runID = RunID(r)
		RunWithRetry(WithNewTrace(context.Background()), r, "scenario", func(r Reporter) {
			RunWithRetry(context.Background(), r, "step", func(r Reporter) {}, nil)
		}, nil)
	}, WithTracerProvider(tp))

	spans := rec.Ended()
	root := spanByName(t, spans, "run")
	scenario := spanByName(t, spans, "scenario")
	step := spanByName(t, spans, "step")

	if scenario.SpanContext().TraceID() == root.SpanContext().TraceID() {
		t.Error("scenario must start a new trace")
	}
	if scenario.Parent().IsValid() {
		t.Error("scenario must be a root span")
	}
	links := scenario.Links()
	if len(links) != 1 || links[0].SpanContext.SpanID() != root.SpanContext().SpanID() {
		t.Errorf("scenario must link to the run span, got %+v", links)
	}
	if v, ok := attrValue(scenario, "scenarigo.run_id"); !ok || v.AsString() != runID {
		t.Errorf("scenario scenarigo.run_id = %q, want %q", v.AsString(), runID)
	}
	if step.SpanContext().TraceID() != scenario.SpanContext().TraceID() {
		t.Error("step must join the scenario trace, not start another one")
	}
	if step.Parent().SpanID() != scenario.SpanContext().SpanID() {
		t.Error("step must be a child of the scenario span")
	}
}

func TestNodeSpans_Serializable(t *testing.T) {
	tp, _ := newRecordingProvider(t)
	Run(func(r Reporter) {
		r.Run("node", func(r Reporter) {
			SetNodeKind(r, NodeKindStep)
			SetStepIndex(r, 1)
			rep := r.(*reporter)
			sc := rep.Span().SpanContext()

			m := map[string]*SerializableReporter{}
			id := rep.ToSerializable(m)
			sr := m[id]
			if sr.TraceID != sc.TraceID().String() || sr.SpanID != sc.SpanID().String() {
				t.Errorf("serialized span context mismatch: %+v", sr)
			}
			if sr.TraceFlags != "01" {
				t.Errorf("serialized trace flags = %q, want 01 (sampled)", sr.TraceFlags)
			}
			if sr.RunID != RunID(r) || sr.NodeKind != "step" || sr.StepIndex == nil || *sr.StepIndex != 1 {
				t.Errorf("serialized metadata mismatch: %+v", sr)
			}

			// Round-trip through JSON as the WASM boundary does.
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]*SerializableReporter
			if err := json.Unmarshal(b, &decoded); err != nil {
				t.Fatal(err)
			}
			restored := FromSerializable(id, decoded)
			if !restored.remote {
				t.Error("restored reporter must be flagged remote")
			}
			rsc := restored.Span().SpanContext()
			if rsc.TraceID() != sc.TraceID() || rsc.SpanID() != sc.SpanID() || !rsc.IsRemote() || !rsc.IsSampled() {
				t.Errorf("restored span context mismatch: %+v", rsc)
			}
			if restored.Span().IsRecording() {
				t.Error("restored span must not record")
			}
			if restored.getRunID() != RunID(r) {
				t.Errorf("restored run ID = %q, want %q", restored.getRunID(), RunID(r))
			}
			if restored.getSpanID() != sc.SpanID().String() {
				t.Errorf("restored span ID = %q", restored.getSpanID())
			}

			// A node created on the guest side has no span of its own and borrows the ancestor's.
			var guestChild Reporter
			restored.Run("guest", func(r Reporter) { guestChild = r })
			if guestChild.Span().SpanContext().SpanID() != sc.SpanID() {
				t.Error("guest child must return the nearest serialized span")
			}
			if guestChild.(*reporter).getSpanID() != "" {
				t.Error("guest child must not report a span ID of its own")
			}
			if guestChild.(*reporter).getTraceID() != sc.TraceID().String() {
				t.Error("guest child must report the trace ID of the serialized span")
			}
		})
	}, WithTracerProvider(tp))
}

func TestNodeSpans_NoopProvider(t *testing.T) {
	var span trace.Span
	var runID string
	Run(func(r Reporter) {
		runID = RunID(r)
		span = r.Span()
	}, WithTracerProvider(noopTracerProvider{}))
	if runID != "" {
		t.Errorf("run ID must be empty without valid spans, got %q", runID)
	}
	if span == nil || span.SpanContext().IsValid() {
		t.Error("span must be a no-op span with an invalid context")
	}
}

type noopTracerProvider struct{ trace.TracerProvider }

func (noopTracerProvider) Tracer(string, ...trace.TracerOption) trace.Tracer {
	return trace.SpanFromContext(context.Background()).TracerProvider().Tracer("")
}

func TestGenerateTestReport_IDs(t *testing.T) {
	tp, rec := newRecordingProvider(t)
	var report *TestReport
	Run(func(r Reporter) {
		r.Run("file.yaml", func(r Reporter) {
			SetNodeKind(r, NodeKindFile)
			RunWithRetry(WithNewTrace(context.Background()), r, "scenario", func(r Reporter) {
				SetNodeKind(r, NodeKindScenario)
				SetScenarioIndex(r, 1)
				r.Run("step", func(r Reporter) {
					SetNodeKind(r, NodeKindStep)
					SetStepIndex(r, 0)
					r.Run("substep", func(r Reporter) {})
				})
			}, nil)
		})
		r.Cleanup(func() {
			var err error
			report, err = GenerateTestReport(r)
			if err != nil {
				t.Errorf("failed to generate report: %s", err)
			}
		})
	}, WithTracerProvider(tp))

	spans := rec.Ended()
	run := spanByName(t, spans, "run")
	file := spanByName(t, spans, "file.yaml")
	scenario := spanByName(t, spans, "scenario")
	step := spanByName(t, spans, "step")
	substep := spanByName(t, spans, "substep")

	if report.RunID != run.SpanContext().TraceID().String() {
		t.Errorf("runId = %q, want the run trace ID", report.RunID)
	}
	f := report.Files[0]
	if f.SpanID != file.SpanContext().SpanID().String() || f.RunID != report.RunID {
		t.Errorf("file report ids: %+v", f)
	}
	s := f.Scenarios[0]
	if s.TraceID != scenario.SpanContext().TraceID().String() || s.SpanID != scenario.SpanContext().SpanID().String() {
		t.Errorf("scenario report ids: traceId=%s spanId=%s", s.TraceID, s.SpanID)
	}
	if s.TraceID == report.RunID {
		t.Error("scenario trace must differ from the run trace")
	}
	if s.ScenarioIndex == nil || *s.ScenarioIndex != 1 {
		t.Errorf("scenarioIndex = %v", s.ScenarioIndex)
	}
	st := s.Steps[0]
	if st.SpanID != step.SpanContext().SpanID().String() || st.StepIndex == nil || *st.StepIndex != 0 {
		t.Errorf("step report: %+v", st)
	}
	ss := st.SubSteps[0]
	if ss.SpanID != substep.SpanContext().SpanID().String() || ss.ScenarioIndex != nil || ss.StepIndex != nil {
		t.Errorf("substep report: %+v", ss)
	}

	// JUnit: run ID as a testsuite property, trace ID and index as testcase attributes.
	b, err := xml.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<property name="scenarigo.run_id" value="` + report.RunID + `"></property>`,
		`trace-id="` + s.TraceID + `"`,
		`scenario-index="1"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JUnit output lacks %s:\n%s", want, b)
		}
	}
}

// Scenario nodes set their indices after Parallel(), while a sibling's WASM
// call may serialize the whole tree at the same time.
func TestNodeIndices_ConcurrentSerialization(t *testing.T) {
	Run(func(r Reporter) {
		r.Run("file", func(r Reporter) {
			r.Parallel()
			for i := range 4 {
				r.Run("scenario", func(r Reporter) {
					r.Parallel()
					for range 50 {
						SetNodeKind(r, NodeKindScenario)
						SetScenarioIndex(r, i)
						SetStepIndex(r, i)
						r.(*reporter).ToSerializable(map[string]*SerializableReporter{})
					}
				})
			}
		})
	}, WithMaxParallel(8))
}

func TestChildren_ConcurrentSerialization(t *testing.T) {
	Run(func(r Reporter) {
		r.Run("reader", func(r Reporter) {
			r.Parallel()
			for range 200 {
				r.(*reporter).ToSerializable(map[string]*SerializableReporter{})
			}
		})
		r.Run("writer", func(r Reporter) {
			r.Parallel()
			for range 50 {
				r.Run("scenario", func(r Reporter) { r.Parallel() })
			}
		})
	}, WithMaxParallel(8))
}
