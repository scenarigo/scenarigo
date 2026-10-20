package protocolmeta

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
	grpccodes "google.golang.org/grpc/codes"
)

// Instrumentation scope names of the request spans.
const (
	HTTPTracerName = "github.com/scenarigo/scenarigo/protocol/http"
	GRPCTracerName = "github.com/scenarigo/scenarigo/protocol/grpc"
)

// HTTPRequestAttributes returns the attributes of an HTTP request span.
// url.full, the query and the body stay out: they carry secrets more often than not.
func HTTPRequestAttributes(runID string, req *http.Request) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 6)
	if runID != "" {
		attrs = append(attrs, AttrRunID.String(runID))
	}
	attrs = append(attrs, semconv.HTTPRequestMethodKey.String(req.Method))
	if req.URL == nil {
		return attrs
	}
	if host := req.URL.Hostname(); host != "" {
		attrs = append(attrs, semconv.ServerAddress(host))
	}
	if port := serverPort(req.URL.Port(), req.URL.Scheme); port > 0 {
		attrs = append(attrs, semconv.ServerPort(port))
	}
	if req.URL.Scheme != "" {
		attrs = append(attrs, semconv.URLScheme(req.URL.Scheme))
	}
	if req.URL.Path != "" {
		attrs = append(attrs, semconv.URLPath(req.URL.Path))
	}
	return attrs
}

func serverPort(port, scheme string) int {
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil {
			return 0
		}
		return n
	}
	switch scheme {
	case "http":
		return 80
	case "https":
		return 443
	}
	return 0
}

// RecordHTTPResponse records the status code. 4xx and 5xx mark the span as
// failed with error.type set to the status code, as the HTTP client
// conventions prescribe.
func RecordHTTPResponse(span trace.Span, statusCode int) {
	span.SetAttributes(semconv.HTTPResponseStatusCode(statusCode))
	if statusCode >= http.StatusBadRequest {
		span.SetAttributes(semconv.ErrorTypeKey.String(strconv.Itoa(statusCode)))
		span.SetStatus(codes.Error, "")
	}
}

// GRPCRequestAttributes returns the attributes of a gRPC request span.
// method is the full method name ("/package.Service/Method") when known, or
// the bare method name. The leading slash is the wire form; the semantic
// conventions spell rpc.method as "package.Service/Method".
func GRPCRequestAttributes(runID, method string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 3)
	if runID != "" {
		attrs = append(attrs, AttrRunID.String(runID))
	}
	attrs = append(attrs, semconv.RPCSystemNameKey.String("grpc"))
	if method = strings.TrimPrefix(method, "/"); method != "" {
		attrs = append(attrs, semconv.RPCMethod(method))
	}
	return attrs
}

// MaskAttributes applies replace to every string attribute and leaves out an
// integer attribute whose decimal form replace would change (a port taken
// from a secret). A value templated from secrets must not reach the trace
// backend while the logs mask it. Attributes of other types stay as they are.
func MaskAttributes(attrs []attribute.KeyValue, replace func(string) string) []attribute.KeyValue {
	masked := make([]attribute.KeyValue, 0, len(attrs))
	for _, kv := range attrs {
		switch kv.Value.Type() {
		case attribute.STRING:
			kv = kv.Key.String(replace(kv.Value.AsString()))
		case attribute.INT64:
			if s := strconv.FormatInt(kv.Value.AsInt64(), 10); replace(s) != s {
				continue
			}
		default:
		}
		masked = append(masked, kv)
	}
	return masked
}

// RecordGRPCStatus records the response status. Every code but OK marks the
// span as failed with error.type set to the code name.
func RecordGRPCStatus(span trace.Span, code grpccodes.Code) {
	span.SetAttributes(semconv.RPCResponseStatusCode(code.String()))
	if code != grpccodes.OK {
		span.SetAttributes(semconv.ErrorTypeKey.String(code.String()))
		span.SetStatus(codes.Error, "")
	}
}

// RecordSendError marks the span as failed by an error outside the response
// status: metadata build, transport, or reading the response body. error.type
// is the Go type of err: low cardinality, and never the message.
func RecordSendError(span trace.Span, err error) {
	span.SetAttributes(semconv.ErrorTypeKey.String(fmt.Sprintf("%T", err)))
	span.SetStatus(codes.Error, "")
}
