package schema

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLoadConfig_Telemetry(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg, err := LoadConfig("testdata/config/valid.yaml")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if diff := cmp.Diff(TelemetryConfig{}, cfg.Telemetry); diff != "" {
			t.Errorf("telemetry must be zero when absent (-want +got):\n%s", diff)
		}
	})
	t.Run("configured", func(t *testing.T) {
		cfg, err := LoadConfig("testdata/config/telemetry.yaml")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		f, tr := false, true
		expect := TelemetryConfig{
			Tracing:     TracingConfig{Exporter: "otlp"},
			Propagation: PropagationConfig{Traceparent: &f, Baggage: &tr, LegacyHeaders: &f},
		}
		if diff := cmp.Diff(expect, cfg.Telemetry); diff != "" {
			t.Errorf("differs (-want +got):\n%s", diff)
		}
	})
	t.Run("unknown exporter", func(t *testing.T) {
		_, err := LoadConfig("testdata/config/invalid-telemetry-exporter.yaml")
		if err == nil || !strings.Contains(err.Error(), `unknown exporter "jaeger"`) {
			t.Fatalf("expected an unknown exporter error, got %v", err)
		}
	})
	t.Run("sampler is reserved", func(t *testing.T) {
		_, err := LoadConfig("testdata/config/invalid-telemetry-sampler.yaml")
		if err == nil || !strings.Contains(err.Error(), "sampler") {
			t.Fatalf("expected a strict decoding error naming sampler, got %v", err)
		}
	})
}
