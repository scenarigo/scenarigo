package protocolmeta

import (
	"strconv"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func endedSpan(t *testing.T, record func(trace.Span)) sdktrace.ReadOnlySpan {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	_, span := tp.Tracer("test").Start(t.Context(), "request")
	record(span)
	span.End()
	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 span, got %d", len(ended))
	}
	return ended[0]
}

func spanAttr(s sdktrace.ReadOnlySpan, key attribute.Key) (attribute.Value, bool) {
	for _, kv := range s.Attributes() {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestRecordHTTPResponse_ErrorBoundary(t *testing.T) {
	tests := map[string]struct {
		status  int
		wantErr bool
	}{
		"200": {status: 200},
		"399": {status: 399},
		"400": {status: 400, wantErr: true},
		"404": {status: 404, wantErr: true},
		"500": {status: 500, wantErr: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			s := endedSpan(t, func(span trace.Span) { RecordHTTPResponse(span, test.status) })
			if v, ok := spanAttr(s, "http.response.status_code"); !ok || v.AsInt64() != int64(test.status) {
				t.Errorf("http.response.status_code = %v (present=%t)", v.AsInt64(), ok)
			}
			if gotErr := s.Status().Code == codes.Error; gotErr != test.wantErr {
				t.Errorf("status = %v, want error=%t", s.Status().Code, test.wantErr)
			}
			v, ok := spanAttr(s, "error.type")
			if ok != test.wantErr {
				t.Errorf("error.type present=%t, want %t", ok, test.wantErr)
			}
			if test.wantErr && v.AsString() != strconv.Itoa(test.status) {
				t.Errorf("error.type = %q", v.AsString())
			}
		})
	}
}

func TestGRPCRequestAttributes_Method(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"full method name loses the wire slash": {in: "/pkg.Service/Method", want: "pkg.Service/Method"},
		"bare method":                           {in: "Echo", want: "Echo"},
		"unknown":                               {in: "", want: ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var got string
			var present bool
			for _, kv := range GRPCRequestAttributes("run", test.in) {
				if kv.Key == "rpc.method" {
					got, present = kv.Value.AsString(), true
				}
			}
			if present != (test.want != "") || got != test.want {
				t.Errorf("rpc.method = %q (present=%t), want %q", got, present, test.want)
			}
		})
	}
}

func TestMaskAttributes(t *testing.T) {
	replace := func(s string) string {
		s = strings.ReplaceAll(s, "SECRET", "{{secrets.token}}")
		return strings.ReplaceAll(s, "8443", "{{secrets.port}}")
	}
	attrs := []attribute.KeyValue{
		attribute.String("url.path", "/webhooks/SECRET"),
		attribute.Int("server.port", 443),
		attribute.Int("http.response.status_code", 8443),
		attribute.Bool("flag", true),
	}
	got := MaskAttributes(attrs, replace)
	if len(got) != 3 {
		t.Fatalf("expected 3 attributes (the secret integer left out), got %v", got)
	}
	if got[0].Value.AsString() != "/webhooks/{{secrets.token}}" {
		t.Errorf("url.path = %q", got[0].Value.AsString())
	}
	if got[1].Key != "server.port" || got[1].Value.AsInt64() != 443 {
		t.Errorf("server.port = %v, must be left alone", got[1].Value.String())
	}
	if got[2].Key != "flag" {
		t.Errorf("got[2] = %s, want the bool attribute", got[2].Key)
	}
}
