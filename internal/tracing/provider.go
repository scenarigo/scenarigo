// Package tracing builds the OpenTelemetry TracerProvider the scenarigo command runs with.
package tracing

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"

	"github.com/scenarigo/scenarigo/schema"
	"github.com/scenarigo/scenarigo/version"
)

// Exporter names accepted by telemetry.tracing.exporter; schema validates them.
const (
	ExporterNone = schema.TracingExporterNone
	ExporterOTLP = schema.TracingExporterOTLP
)

// ExporterEnv is the OpenTelemetry environment variable that selects the
// traces exporter. The configuration file is shared through the repository,
// so this per-environment value wins over telemetry.tracing.exporter: CI can
// turn tracing on without a local run also trying to reach a collector.
const ExporterEnv = "OTEL_TRACES_EXPORTER"

// ResolveExporter returns the exporter named by ExporterEnv, or the configured
// one when the variable is unset or empty. The variable is matched exactly
// against ExporterNone and ExporterOTLP: the comma-separated lists and the other
// exporters the OpenTelemetry specification allows are an error here.
func ResolveExporter(configured string) (string, error) {
	switch env := os.Getenv(ExporterEnv); env {
	case "":
		return configured, nil
	case ExporterNone, ExporterOTLP:
		return env, nil
	default:
		return "", fmt.Errorf("unsupported exporter %q in %s: use %s or %s", env, ExporterEnv, ExporterNone, ExporterOTLP)
	}
}

// NewTracerProvider returns the provider for the given exporter name, or nil
// for "" and "none": the reporter then uses its own processor-less provider.
// The OTLP exporter is configured by the OTEL_EXPORTER_OTLP_* environment
// variables; the protocol is http/protobuf unless the environment selects grpc.
// Every span is sampled: a scenario run is small and the IDs on the wire must
// resolve to exported spans.
func NewTracerProvider(ctx context.Context, exporter string) (*sdktrace.TracerProvider, error) {
	switch exporter {
	case "", ExporterNone:
		return nil, nil //nolint:nilnil // no provider means tracing is off; the reporter uses its own
	case ExporterOTLP:
	default:
		return nil, fmt.Errorf("unknown exporter %q", exporter)
	}
	exp, err := newOTLPExporter(ctx)
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("scenarigo"),
			semconv.ServiceVersionKey.String(version.String()),
		),
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(), // OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES win
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build resource: %w", err)
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	), nil
}

func newOTLPExporter(ctx context.Context) (*otlptrace.Exporter, error) {
	protocol := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL")
	if protocol == "" {
		protocol = os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	}
	switch protocol {
	case "", "http/protobuf":
		return otlptracehttp.New(ctx)
	case "grpc":
		return otlptracegrpc.New(ctx)
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol %q: use http/protobuf or grpc", protocol)
	}
}

// Shutdown flushes and shuts the provider down under one deadline.
func Shutdown(tp *sdktrace.TracerProvider, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	flushErr := tp.ForceFlush(ctx)
	if err := tp.Shutdown(ctx); err != nil && flushErr == nil {
		return err
	}
	return flushErr
}
