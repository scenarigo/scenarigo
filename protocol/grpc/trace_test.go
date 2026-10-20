package grpc

import (
	gocontext "context"
	"io"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/mockutil"
	"github.com/scenarigo/scenarigo/internal/protocolmeta"
	"github.com/scenarigo/scenarigo/reporter"
	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

// ignoreW3CMetadata drops the metadata whose values change on every run.
var ignoreW3CMetadata = cmpopts.IgnoreMapEntries(func(k string, _ []string) bool {
	return k == protocolmeta.TraceparentHeader || k == protocolmeta.BaggageHeader
})

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
	req := &testpb.EchoRequest{MessageId: "1", MessageBody: "hello"}
	resp := &testpb.EchoResponse{MessageId: "1", MessageBody: "hello"}

	newRequest := func() *Request {
		return &Request{
			Client: "{{vars.client}}",
			Method: "Echo",
			Message: yaml.MapSlice{
				yaml.MapItem{Key: "messageId", Value: "1"},
				yaml.MapItem{Key: "messageBody", Value: "hello"},
			},
			Metadata: map[string]string{
				"baggage":     "user=1",
				"traceparent": "00-11111111111111111111111111111111-2222222222222222-01",
				"tracestate":  "vendor=x",
			},
		}
	}
	setup := func(t *testing.T, err error) (*context.Context, reporter.Reporter, *tracetest.SpanRecorder, *metadata.MD) {
		t.Helper()
		ctrl := gomock.NewController(t)
		client := testpb.NewMockTestClient(ctrl)
		var received metadata.MD
		client.EXPECT().Echo(gomock.Any(), mockutil.ProtoMessage(req), gomock.Any()).DoAndReturn(
			func(ctx gocontext.Context, _ *testpb.EchoRequest, _ ...grpc.CallOption) (*testpb.EchoResponse, error) {
				received, _ = metadata.FromOutgoingContext(ctx)
				if err != nil {
					return nil, err
				}
				return resp, nil
			})
		r, spans := recordingReporter(t)
		ctx := context.New(r).WithRunID("run-1").WithScenarioFilepath("s.yaml").WithScenarioIndex(2).WithStepIndex(0).
			WithVars(map[string]any{"client": client})
		return ctx, r, spans, &received
	}

	t.Run("ok", func(t *testing.T) {
		ctx, r, spans, received := setup(t, nil)
		if _, _, err := newRequest().Invoke(ctx); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		ended := spans.Ended()
		if len(ended) != 1 {
			t.Fatalf("expected 1 request span, got %d", len(ended))
		}
		span := ended[0]
		if span.SpanKind() != trace.SpanKindClient || span.Name() != "Echo" {
			t.Errorf("span = %s/%v", span.Name(), span.SpanKind())
		}
		if span.Parent().SpanID() != r.Span().SpanContext().SpanID() {
			t.Error("request span must be a child of the node span")
		}
		if span.Status().Code != codes.Unset {
			t.Errorf("status = %v", span.Status().Code)
		}
		for k, v := range map[attribute.Key]string{"scenarigo.run_id": "run-1", "rpc.system.name": "grpc", "rpc.method": "Echo", "rpc.response.status_code": "OK"} {
			if got, ok := attrValue(span, k); !ok || got.String() != v {
				t.Errorf("%s = %q (present=%t), want %q", k, got.String(), ok, v)
			}
		}

		md := *received
		tp := md.Get("traceparent")
		if len(tp) != 1 || !strings.Contains(tp[0], span.SpanContext().SpanID().String()) {
			t.Errorf("traceparent %v must name the request span", tp)
		}
		if len(md.Get("tracestate")) != 0 {
			t.Errorf("user tracestate must be dropped, got %v", md.Get("tracestate"))
		}
		bags := md.Get("baggage")
		if len(bags) != 1 {
			t.Fatalf("baggage must be one value, got %v", bags)
		}
		bag, err := baggage.Parse(bags[0])
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range map[string]string{"user": "1", "scenarigo.run_id": "run-1", "scenarigo.filepath": "s.yaml", "scenarigo.scenario_index": "2", "scenarigo.step_index": "0"} {
			if bag.Member(k).Value() != v {
				t.Errorf("baggage %s = %q, want %q", k, bag.Member(k).Value(), v)
			}
		}
		if len(md.Get(protocolmeta.ScenarigoScenarioFilepathBinKey)) == 0 {
			t.Error("legacy metadata must be written by default")
		}
	})

	t.Run("non-OK status is an error", func(t *testing.T) {
		ctx, _, spans, _ := setup(t, status.Error(grpccodes.NotFound, "missing"))
		if _, _, err := newRequest().Invoke(ctx); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		span := spans.Ended()[0]
		if span.Status().Code != codes.Error || span.Status().Description != "" {
			t.Errorf("status = %+v", span.Status())
		}
		if v, ok := attrValue(span, "rpc.response.status_code"); !ok || v.AsString() != "NotFound" {
			t.Errorf("rpc.response.status_code = %q", v.AsString())
		}
		if v, ok := attrValue(span, "error.type"); !ok || v.AsString() != "NotFound" {
			t.Errorf("error.type = %q", v.AsString())
		}
	})

	t.Run("propagation off", func(t *testing.T) {
		ctx, _, _, received := setup(t, nil)
		ctx = ctx.WithTelemetryConfig(context.TelemetryConfig{})
		if _, _, err := newRequest().Invoke(ctx); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		md := *received
		if got := md.Get("traceparent"); len(got) != 1 || got[0] != "00-11111111111111111111111111111111-2222222222222222-01" {
			t.Errorf("user traceparent must be left alone, got %v", got)
		}
		if got := md.Get("tracestate"); len(got) != 1 {
			t.Errorf("user tracestate must be left alone, got %v", got)
		}
		if got := md.Get("baggage"); len(got) != 1 || got[0] != "user=1" {
			t.Errorf("user baggage must be left alone, got %v", got)
		}
		for k := range md {
			if strings.HasPrefix(k, "scenarigo-") {
				t.Errorf("legacy metadata %s must not be written", k)
			}
		}
	})
}

func TestRequest_Invoke_Trace_MetadataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := testpb.NewMockTestClient(ctrl)
	r, spans := recordingReporter(t)
	ctx := context.New(r).WithVars(map[string]any{"client": client})
	req := &Request{Client: "{{vars.client}}", Method: "Echo", Metadata: map[string]string{"key": "{{vars.missing}}"}}
	if _, _, err := req.Invoke(ctx); err == nil {
		t.Fatal("expected an error")
	}
	ended := spans.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected the request span to end, got %d spans", len(ended))
	}
	if ended[0].Status().Code != codes.Error {
		t.Errorf("a request that was never sent must be marked as failed, got %v", ended[0].Status().Code)
	}
}

// outgoingMD matches any context and keeps its outgoing metadata.
type outgoingMD struct{ md *metadata.MD }

func (m outgoingMD) Matches(x any) bool {
	if ctx, ok := x.(gocontext.Context); ok {
		*m.md, _ = metadata.FromOutgoingContext(ctx)
	}
	return true
}

func (outgoingMD) String() string { return "any context" }

func TestRequest_Invoke_Trace_ServerStream(t *testing.T) {
	req := &testpb.EchoRequest{MessageId: "1", MessageBody: "hello"}
	resp := &testpb.EchoResponse{MessageId: "1", MessageBody: "hello"}
	setup := func(t *testing.T, recvErr error) (*context.Context, reporter.Reporter, *tracetest.SpanRecorder, *metadata.MD) {
		t.Helper()
		ctrl := gomock.NewController(t)
		stream := testpb.NewMockTest_ServerStreamEchoClient(ctrl)
		stream.EXPECT().Recv().Return(resp, nil)
		if recvErr != nil {
			stream.EXPECT().Recv().Return(nil, recvErr)
		} else {
			stream.EXPECT().Recv().Return(nil, io.EOF)
		}
		stream.EXPECT().Header().Return(metadata.MD{}, nil).AnyTimes()
		stream.EXPECT().Trailer().Return(metadata.MD{}).AnyTimes()
		var received metadata.MD
		client := testpb.NewMockTestClient(ctrl)
		client.EXPECT().ServerStreamEcho(outgoingMD{&received}, mockutil.ProtoMessage(req), gomock.Any()).Return(stream, nil)
		r, spans := recordingReporter(t)
		ctx := context.New(r).WithRunID("run-1").WithScenarioFilepath("s.yaml").WithScenarioIndex(0).WithStepIndex(3).
			WithVars(map[string]any{"client": client})
		return ctx, r, spans, &received
	}
	newRequest := func() *Request {
		return &Request{
			Client: "{{vars.client}}",
			Method: "ServerStreamEcho",
			Message: yaml.MapSlice{
				yaml.MapItem{Key: "messageId", Value: "1"},
				yaml.MapItem{Key: "messageBody", Value: "hello"},
			},
		}
	}

	t.Run("ok", func(t *testing.T) {
		ctx, r, spans, received := setup(t, nil)
		if _, _, err := newRequest().Invoke(ctx); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		ended := spans.Ended()
		if len(ended) != 1 {
			t.Fatalf("expected 1 request span, got %d", len(ended))
		}
		span := ended[0]
		if span.SpanKind() != trace.SpanKindClient || span.Name() != "ServerStreamEcho" {
			t.Errorf("span = %s/%v", span.Name(), span.SpanKind())
		}
		if span.Parent().SpanID() != r.Span().SpanContext().SpanID() {
			t.Error("request span must be a child of the node span")
		}
		if span.Status().Code != codes.Unset {
			t.Errorf("status = %v", span.Status().Code)
		}
		if v, ok := attrValue(span, "rpc.response.status_code"); !ok || v.AsString() != "OK" {
			t.Errorf("rpc.response.status_code = %q", v.AsString())
		}
		md := *received
		if tp := md.Get("traceparent"); len(tp) != 1 || !strings.Contains(tp[0], span.SpanContext().SpanID().String()) {
			t.Errorf("traceparent %v must name the request span", tp)
		}
		bags := md.Get("baggage")
		if len(bags) != 1 {
			t.Fatalf("baggage must be one value, got %v", bags)
		}
		bag, err := baggage.Parse(bags[0])
		if err != nil {
			t.Fatal(err)
		}
		if bag.Member("scenarigo.step_index").Value() != "3" || bag.Member("scenarigo.run_id").Value() != "run-1" {
			t.Errorf("baggage = %q", bags[0])
		}
	})

	t.Run("stream error", func(t *testing.T) {
		ctx, _, spans, _ := setup(t, status.Error(grpccodes.NotFound, "missing"))
		_, _, _ = newRequest().Invoke(ctx)
		ended := spans.Ended()
		if len(ended) != 1 {
			t.Fatalf("expected 1 request span, got %d", len(ended))
		}
		if ended[0].Status().Code != codes.Error {
			t.Errorf("status = %v", ended[0].Status().Code)
		}
		if _, ok := attrValue(ended[0], "error.type"); !ok {
			t.Error("error.type must be set")
		}
	})
}
