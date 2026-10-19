package protocolmeta

import (
	"net/http"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc/metadata"
)

// Carrier is a propagation.TextMapCarrier that can also report every value of a
// key and delete a key. Both are needed to replace a user-supplied traceparent
// together with its tracestate and to inspect the raw baggage before parsing.
type Carrier interface {
	propagation.TextMapCarrier
	Values(key string) []string
	Delete(key string)
}

// HeaderCarrier adapts http.Header to Carrier.
type HeaderCarrier http.Header

var _ Carrier = HeaderCarrier{}

// Get implements propagation.TextMapCarrier.
func (c HeaderCarrier) Get(key string) string { return http.Header(c).Get(key) }

// Set implements propagation.TextMapCarrier.
func (c HeaderCarrier) Set(key, value string) { http.Header(c).Set(key, value) }

// Keys implements propagation.TextMapCarrier.
func (c HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// Values implements Carrier and propagation.ValuesGetter.
func (c HeaderCarrier) Values(key string) []string { return http.Header(c).Values(key) }

// Delete implements Carrier.
func (c HeaderCarrier) Delete(key string) { http.Header(c).Del(key) }

// MDCarrier adapts gRPC metadata.MD to Carrier.
type MDCarrier metadata.MD

var _ Carrier = MDCarrier{}

// Get implements propagation.TextMapCarrier.
func (c MDCarrier) Get(key string) string {
	vs := metadata.MD(c).Get(key)
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}

// Set implements propagation.TextMapCarrier.
func (c MDCarrier) Set(key, value string) { metadata.MD(c).Set(key, value) }

// Keys implements propagation.TextMapCarrier.
func (c MDCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// Values implements Carrier and propagation.ValuesGetter.
func (c MDCarrier) Values(key string) []string { return metadata.MD(c).Get(key) }

// Delete implements Carrier.
func (c MDCarrier) Delete(key string) { metadata.MD(c).Delete(key) }
