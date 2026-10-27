package tracing

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/scenarigo/scenarigo/reporter"
)

func TestNewTracerProvider(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		for _, name := range []string{"", "none"} {
			tp, err := NewTracerProvider(context.Background(), name)
			if err != nil {
				t.Fatalf("%q: unexpected error: %s", name, err)
			}
			if tp != nil {
				t.Errorf("%q: provider must be nil", name)
			}
		}
	})
	t.Run("unknown exporter", func(t *testing.T) {
		if _, err := NewTracerProvider(context.Background(), "jaeger"); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("unsupported protocol", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/json")
		_, err := NewTracerProvider(context.Background(), "otlp")
		if err == nil || !strings.Contains(err.Error(), "http/json") {
			t.Fatalf("expected an error naming the protocol, got %v", err)
		}
	})
	t.Run("traces protocol wins over the generic one", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/json")
		if _, err := NewTracerProvider(context.Background(), "otlp"); err == nil {
			t.Fatal("expected an error from the traces protocol")
		}
	})
}

func TestResolveExporter(t *testing.T) {
	tests := map[string]struct {
		env        string
		configured string
		want       string
		wantErr    string
	}{
		"unset keeps the configured otlp":      {configured: "otlp", want: "otlp"},
		"unset keeps the configured none":      {configured: "none", want: "none"},
		"unset keeps the empty configuration":  {configured: "", want: ""},
		"env otlp turns tracing on":            {env: "otlp", configured: "none", want: "otlp"},
		"env otlp without configuration":       {env: "otlp", configured: "", want: "otlp"},
		"env none turns tracing off":           {env: "none", configured: "otlp", want: "none"},
		"env does not validate the configured": {env: "none", configured: "jaeger", want: "none"},
		"unsupported exporter":                 {env: "console", configured: "none", wantErr: `"console"`},
		"several exporters":                    {env: "otlp,console", configured: "none", wantErr: `"otlp,console"`},
		"surrounding whitespace":               {env: " otlp", configured: "none", wantErr: `" otlp"`},
		"upper case":                           {env: "OTLP", configured: "none", wantErr: `"OTLP"`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.env == "" {
				t.Setenv(ExporterEnv, "") // registers the restore
				if err := os.Unsetenv(ExporterEnv); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv(ExporterEnv, test.env)
			}
			got, err := ResolveExporter(test.configured)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) || !strings.Contains(err.Error(), ExporterEnv) {
					t.Fatalf("expected an error naming %s and the value %s, got %v", ExporterEnv, test.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
	t.Run("set but empty counts as unset", func(t *testing.T) {
		t.Setenv(ExporterEnv, "")
		got, err := ResolveExporter("otlp")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if got != "otlp" {
			t.Errorf("got %q, want %q", got, "otlp")
		}
	})
}

// isolateOTELEnv unsets the OTEL_* variables the OTLP exporters and the
// resource detector read, so the caller's shell does not steer the test.
func isolateOTELEnv(t *testing.T) {
	t.Helper()
	keys := []string{"OTEL_RESOURCE_ATTRIBUTES", "OTEL_SERVICE_NAME"}
	for _, suffix := range []string{"ENDPOINT", "PROTOCOL", "HEADERS", "TIMEOUT", "COMPRESSION", "CERTIFICATE", "CLIENT_CERTIFICATE", "CLIENT_KEY", "INSECURE"} {
		keys = append(keys, "OTEL_EXPORTER_OTLP_"+suffix, "OTEL_EXPORTER_OTLP_TRACES_"+suffix)
	}
	for _, key := range keys {
		t.Setenv(key, "") // registers the restore
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

type otlpServer struct {
	mu       sync.Mutex
	requests []*coltracepb.ExportTraceServiceRequest
}

func (s *otlpServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/v1/traces" || req.Header.Get("Content-Type") != "application/x-protobuf" {
		http.Error(w, "unexpected request", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var export coltracepb.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &export); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, &export)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/x-protobuf")
	resp, _ := proto.Marshal(&coltracepb.ExportTraceServiceResponse{})
	_, _ = w.Write(resp)
}

func (s *otlpServer) spanNames() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := map[string]int{}
	for _, req := range s.requests {
		for _, rs := range req.GetResourceSpans() {
			for _, ss := range rs.GetScopeSpans() {
				for _, span := range ss.GetSpans() {
					names[span.GetName()]++
				}
			}
		}
	}
	return names
}

func (s *otlpServer) resourceAttr(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, req := range s.requests {
		for _, rs := range req.GetResourceSpans() {
			for _, kv := range rs.GetResource().GetAttributes() {
				if kv.GetKey() == key {
					return kv.GetValue().GetStringValue()
				}
			}
		}
	}
	return ""
}

func TestNewTracerProvider_OTLPExport(t *testing.T) {
	srv := &otlpServer{}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	isolateOTELEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", ts.URL)

	tp, err := NewTracerProvider(context.Background(), "otlp")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if tp == nil {
		t.Fatal("provider must not be nil")
	}

	reporter.Run(func(r reporter.Reporter) {
		r.Run("scenario", func(r reporter.Reporter) {
			r.Run("step", func(r reporter.Reporter) {})
		})
	}, reporter.WithTracerProvider(tp))

	if err := Shutdown(tp, 5*time.Second); err != nil {
		t.Fatalf("shutdown failed: %s", err)
	}
	names := srv.spanNames()
	for _, name := range []string{"run", "scenario", "step"} {
		if names[name] != 1 {
			t.Errorf("span %q exported %d times, want 1 (got %v)", name, names[name], names)
		}
	}
	if got := srv.resourceAttr("service.name"); got != "scenarigo" {
		t.Errorf("service.name = %q", got)
	}
	if got := srv.resourceAttr("service.version"); got == "" {
		t.Error("service.version missing")
	}
	if got := srv.resourceAttr("telemetry.sdk.language"); got != "go" {
		t.Errorf("telemetry.sdk.language = %q", got)
	}
}

func TestNewTracerProvider_ResourceFromEnv(t *testing.T) {
	srv := &otlpServer{}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	isolateOTELEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", ts.URL)
	t.Setenv("OTEL_SERVICE_NAME", "custom")

	tp, err := NewTracerProvider(context.Background(), "otlp")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	reporter.Run(func(r reporter.Reporter) {}, reporter.WithTracerProvider(tp))
	if err := Shutdown(tp, 5*time.Second); err != nil {
		t.Fatalf("shutdown failed: %s", err)
	}
	if got := srv.resourceAttr("service.name"); got != "custom" {
		t.Errorf("OTEL_SERVICE_NAME must win, got service.name = %q", got)
	}
}
