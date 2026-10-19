package http

import (
	gocontext "context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/protocolmeta"
	"github.com/scenarigo/scenarigo/reporter"
)

// ignoreW3CHeaders drops the headers whose values change on every run.
var ignoreW3CHeaders = cmpopts.IgnoreMapEntries(func(k string, _ []string) bool {
	switch k {
	case http.CanonicalHeaderKey(protocolmeta.TraceparentHeader), http.CanonicalHeaderKey(protocolmeta.BaggageHeader):
		return true
	}
	return false
})

type headerRecorder struct {
	mu   sync.Mutex
	last http.Header
}

func (h *headerRecorder) handler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		h.mu.Lock()
		h.last = req.Header.Clone()
		h.mu.Unlock()
		w.WriteHeader(status)
	}
}

func recordingReporter(t *testing.T) (reporter.Reporter, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(gocontext.Background()) })
	return reporter.FromT(t, reporter.WithTracerProvider(tp)), rec
}

func attrValue(s sdktrace.ReadOnlySpan, key attribute.Key) (attribute.Value, bool) {
	for _, kv := range s.Attributes() {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestRequest_Invoke_Trace(t *testing.T) {
	rec := &headerRecorder{}
	m := http.NewServeMux()
	m.HandleFunc("/ok", rec.handler(http.StatusOK))
	m.HandleFunc("/fail", rec.handler(http.StatusInternalServerError))
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)

	newCtx := func(t *testing.T) (*context.Context, reporter.Reporter, *tracetest.SpanRecorder) {
		t.Helper()
		r, spans := recordingReporter(t)
		ctx := context.New(r).WithRunID("run-1").WithScenarioFilepath("s.yaml").WithScenarioTitle("title").
			WithScenarioIndex(0).WithStepIndex(1).WithStepName("step")
		return ctx, r, spans
	}

	t.Run("defaults", func(t *testing.T) {
		ctx, r, spans := newCtx(t)
		req := &Request{Method: http.MethodGet, URL: srv.URL + "/ok?secret=1", Header: map[string]string{"Baggage": "user=1", "Traceparent": "00-11111111111111111111111111111111-2222222222222222-01", "Tracestate": "vendor=x"}}
		if _, _, err := req.Invoke(ctx); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		ended := spans.Ended()
		if len(ended) != 1 {
			t.Fatalf("expected 1 request span, got %d", len(ended))
		}
		span := ended[0]
		if span.SpanKind() != trace.SpanKindClient {
			t.Errorf("span kind = %v", span.SpanKind())
		}
		if span.Name() != http.MethodGet {
			t.Errorf("span name = %q", span.Name())
		}
		if span.Parent().SpanID() != r.Span().SpanContext().SpanID() {
			t.Error("request span must be a child of the node span")
		}
		if span.Status().Code != codes.Unset {
			t.Errorf("status = %v, want Unset", span.Status().Code)
		}

		got := rec.last
		tp := got.Get("Traceparent")
		if !strings.Contains(tp, span.SpanContext().TraceID().String()) || !strings.Contains(tp, span.SpanContext().SpanID().String()) {
			t.Errorf("traceparent %q must name the request span %v", tp, span.SpanContext())
		}
		if got.Get("Tracestate") != "" {
			t.Errorf("user tracestate must be dropped, got %q", got.Get("Tracestate"))
		}
		bag, err := baggage.Parse(got.Get("Baggage"))
		if err != nil {
			t.Fatalf("baggage %q: %s", got.Get("Baggage"), err)
		}
		for k, v := range map[string]string{"user": "1", "scenarigo.run_id": "run-1", "scenarigo.filepath": "s.yaml", "scenarigo.scenario_index": "0", "scenarigo.step_index": "1", "scenarigo.scenario_title": "title", "scenarigo.step_name": "step"} {
			if bag.Member(k).Value() != v {
				t.Errorf("baggage %s = %q, want %q", k, bag.Member(k).Value(), v)
			}
		}
		if got.Get("Scenarigo-Scenario-Filepath") == "" {
			t.Error("legacy headers must be written by default")
		}

		expectAttrs := map[attribute.Key]string{
			"scenarigo.run_id":          "run-1",
			"http.request.method":       "GET",
			"server.address":            "127.0.0.1",
			"url.scheme":                "http",
			"url.path":                  "/ok",
			"http.response.status_code": "200",
		}
		for k, v := range expectAttrs {
			got, ok := attrValue(span, k)
			if !ok || got.String() != v {
				t.Errorf("%s = %q (present=%t), want %q", k, got.String(), ok, v)
			}
		}
		if _, ok := attrValue(span, "server.port"); !ok {
			t.Error("server.port missing")
		}
		for _, kv := range span.Attributes() {
			if strings.Contains(kv.Value.String(), "secret") || kv.Key == "url.full" || kv.Key == "url.query" {
				t.Errorf("attribute %s leaks the query or the full URL", kv.Key)
			}
		}
	})

	t.Run("5xx is an error", func(t *testing.T) {
		ctx, _, spans := newCtx(t)
		req := &Request{Method: http.MethodPost, URL: srv.URL + "/fail"}
		if _, _, err := req.Invoke(ctx); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		span := spans.Ended()[0]
		if span.Status().Code != codes.Error || span.Status().Description != "" {
			t.Errorf("status = %+v, want Error without description", span.Status())
		}
		if v, ok := attrValue(span, "error.type"); !ok || v.AsString() != "500" {
			t.Errorf("error.type = %q", v.AsString())
		}
	})

	t.Run("send error", func(t *testing.T) {
		ctx, _, spans := newCtx(t)
		req := &Request{Method: http.MethodGet, URL: "http://127.0.0.1:1/unreachable"}
		if _, _, err := req.Invoke(ctx); err == nil {
			t.Fatal("expected an error")
		}
		ended := spans.Ended()
		if len(ended) != 1 {
			t.Fatalf("expected the request span to end, got %d spans", len(ended))
		}
		if ended[0].Status().Code != codes.Error {
			t.Errorf("status = %v", ended[0].Status().Code)
		}
		if _, ok := attrValue(ended[0], "error.type"); !ok {
			t.Error("error.type missing")
		}
		if _, ok := attrValue(ended[0], "http.response.status_code"); ok {
			t.Error("no status code without a response")
		}
	})

	t.Run("propagation off", func(t *testing.T) {
		ctx, _, _ := newCtx(t)
		ctx = ctx.WithTelemetryConfig(context.TelemetryConfig{Traceparent: false, Baggage: false, LegacyHeaders: false})
		req := &Request{Method: http.MethodGet, URL: srv.URL + "/ok", Header: map[string]string{"Traceparent": "user", "Tracestate": "vendor=x"}}
		if _, _, err := req.Invoke(ctx); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		got := rec.last
		if got.Get("Traceparent") != "user" || got.Get("Tracestate") != "vendor=x" {
			t.Errorf("user trace headers must be left alone: %v", got)
		}
		if got.Get("Baggage") != "" {
			t.Errorf("baggage must not be written, got %q", got.Get("Baggage"))
		}
		for k := range got {
			if strings.HasPrefix(k, "Scenarigo-") {
				t.Errorf("legacy header %s must not be written", k)
			}
		}
	})

	t.Run("baggage only", func(t *testing.T) {
		ctx, _, _ := newCtx(t)
		ctx = ctx.WithTelemetryConfig(context.TelemetryConfig{Traceparent: false, Baggage: true, LegacyHeaders: true})
		req := &Request{Method: http.MethodGet, URL: srv.URL + "/ok"}
		if _, _, err := req.Invoke(ctx); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		got := rec.last
		if got.Get("Traceparent") != "" {
			t.Errorf("traceparent must not be written, got %q", got.Get("Traceparent"))
		}
		if !strings.Contains(got.Get("Baggage"), "scenarigo.run_id=run-1") {
			t.Errorf("baggage = %q", got.Get("Baggage"))
		}
	})

	t.Run("no span without a reporter provider", func(t *testing.T) {
		rptr := reporter.FromT(t, reporter.WithTracerProvider(noopProvider{}))
		ctx := context.New(rptr).WithRunID("run-1")
		req := &Request{Method: http.MethodGet, URL: srv.URL + "/ok"}
		if _, _, err := req.Invoke(ctx); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if rec.last.Get("Traceparent") != "" {
			t.Errorf("no traceparent without a valid span, got %q", rec.last.Get("Traceparent"))
		}
		if !strings.Contains(rec.last.Get("Baggage"), "scenarigo.run_id=run-1") {
			t.Errorf("baggage is independent of tracing, got %q", rec.last.Get("Baggage"))
		}
	})
}

type noopProvider struct{ trace.TracerProvider }

func (noopProvider) Tracer(string, ...trace.TracerOption) trace.Tracer {
	return trace.SpanFromContext(gocontext.Background()).TracerProvider().Tracer("")
}

func TestRequest_Invoke_Trace_SecretsMasked(t *testing.T) {
	rec := &headerRecorder{}
	srv := httptest.NewServer(rec.handler(http.StatusOK))
	t.Cleanup(srv.Close)
	r, spans := recordingReporter(t)
	ctx := context.New(r).WithSecrets(map[string]any{"token": "SUPERSECRET"})
	req := &Request{Method: http.MethodGet, URL: srv.URL + "/webhooks/SUPERSECRET"}
	if _, _, err := req.Invoke(ctx); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ended := spans.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 request span, got %d", len(ended))
	}
	if v, ok := attrValue(ended[0], "url.path"); !ok || v.AsString() != "/webhooks/{{secrets.token}}" {
		t.Errorf("url.path = %q, the secret must be masked as in the logs", v.AsString())
	}
}

func TestRequest_Invoke_Trace_BodyReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	t.Cleanup(srv.Close)
	r, spans := recordingReporter(t)
	req := &Request{Method: http.MethodGet, URL: srv.URL + "/short"}
	if _, _, err := req.Invoke(context.New(r)); err == nil {
		t.Fatal("expected a body read error")
	}
	ended := spans.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 request span, got %d", len(ended))
	}
	span := ended[0]
	if span.Status().Code != codes.Error {
		t.Errorf("a response that could not be read must mark the span as failed, got %v", span.Status().Code)
	}
	if _, ok := attrValue(span, "error.type"); !ok {
		t.Error("error.type must be set")
	}
	if v, ok := attrValue(span, "http.response.status_code"); !ok || v.AsInt64() != http.StatusOK {
		t.Errorf("http.response.status_code = %v", v.AsInt64())
	}
}

func TestRequest_Invoke_Trace_SecretPortOmitted(t *testing.T) {
	rec := &headerRecorder{}
	srv := httptest.NewServer(rec.handler(http.StatusOK))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	r, spans := recordingReporter(t)
	ctx := context.New(r).WithSecrets(map[string]any{"port": u.Port()})
	req := &Request{Method: http.MethodGet, URL: srv.URL + "/ok"}
	if _, _, err := req.Invoke(ctx); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	span := spans.Ended()[0]
	if v, ok := attrValue(span, "server.port"); ok {
		t.Errorf("server.port = %d, a port taken from a secret must be left out", v.AsInt64())
	}
	if _, ok := attrValue(span, "server.address"); !ok {
		t.Error("server.address must stay")
	}
}
