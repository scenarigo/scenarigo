package scenarigo

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/scenarigo/scenarigo"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
)

func TestFromT(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count == 0 {
			w.WriteHeader(http.StatusNotFound)
		}
		count++
	}))
	t.Setenv("TEST_HTTP_ADDR", srv.Listener.Addr().String())

	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, "report.json")
	r, err := scenarigo.NewRunner(scenarigo.WithConfig(&schema.Config{
		SchemaVersion: "config/v1",
		Scenarios: []string{
			"testdata/testcases/scenarios/retry/step-constant.yaml",
		},
		Output: schema.OutputConfig{
			Report: schema.ReportConfig{
				JSON: schema.JSONReportConfig{
					Filename: reportPath,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("failed to create test runner: %s", err)
	}
	rptr := reporter.FromT(t)
	r.Run(context.New(rptr))
	if err := r.CreateTestReport(rptr); err != nil {
		t.Fatalf("faild to create test report: %s", err)
	}
}
