package cmd

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/scenarigo/scenarigo/internal/testutil"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/cobra"
)

func TestRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()
		w.Header().Add("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	t.Setenv("TEST_ADDR", srv.URL)

	tests := map[string]struct {
		args          []string
		config        string
		setupFlags    func(*testing.T, *cobra.Command)
		expectError   string
		expectOutput  string
		expectReports []string
	}{
		"specify by argument": {
			args: []string{"testdata/scenarios/pass.yaml"},
			expectOutput: strings.TrimPrefix(`
ok  	testdata/scenarios/pass.yaml	0.000s
`, "\n"),
		},
		"use config": {
			args:        []string{},
			config:      "./testdata/scenarigo.yaml",
			expectError: ErrTestFailed.Error(),
			expectOutput: strings.TrimPrefix(`
--- FAIL: scenarios/fail.yaml (0.00s)
    --- FAIL: scenarios/fail.yaml//echo (0.00s)
        --- FAIL: scenarios/fail.yaml//echo/POST_/echo (0.00s)
                request:
                  method: POST
                  url: http://127.0.0.1:12345/echo
                  header:
                    User-Agent:
                    - scenarigo/v1.0.0
                  body:
                    message: request
                response:
                  status: 200 OK
                  statusCode: 200
                  header:
                    Content-Length:
                    - "23"
                    Content-Type:
                    - application/json
                    Date:
                    - Mon, 01 Jan 0001 00:00:00 GMT
                  body:
                    message: request
                elapsed time: 0.000000 sec
                expected "response" but got "request"
                      12 |   expect:
                      13 |     code: 200
                      14 |     body:
                    > 15 |       message: "response"
                                          ^
FAIL
FAIL	scenarios/fail.yaml	0.000s
FAIL
ok  	scenarios/pass.yaml	0.000s
`, "\n"),
		},
		"override config by argument": {
			config: "./testdata/scenarigo.yaml",
			args:   []string{"testdata/scenarios/pass.yaml"},
			expectOutput: strings.TrimPrefix(`
ok  	scenarios/pass.yaml	0.000s
`, "\n"),
		},
		"plugin not found": {
			config:      "./testdata/scenarigo-plugin-not-found.yaml",
			args:        []string{"testdata/scenarios/pass.yaml"},
			expectError: ErrTestFailed.Error(),
			expectOutput: strings.TrimPrefix(`
--- FAIL: setup (0.00s)
    --- FAIL: setup/plugin.so (0.00s)
            failed to open plugin: plugin.Open("/go/src/github.com/scenarigo/scenarigo/cmd/testdata/plugin.so"): realpath failed
FAIL
FAIL	setup	0.000s
FAIL
`, "\n"),
		},
		"print summary": {
			args:        []string{},
			config:      "./testdata/scenarigo-summary.yaml",
			expectError: ErrTestFailed.Error(),
			expectOutput: strings.TrimPrefix(`
--- FAIL: scenarios/fail.yaml (0.00s)
    --- FAIL: scenarios/fail.yaml//echo (0.00s)
        --- FAIL: scenarios/fail.yaml//echo/POST_/echo (0.00s)
                request:
                  method: POST
                  url: http://127.0.0.1:12345/echo
                  header:
                    User-Agent:
                    - scenarigo/v1.0.0
                  body:
                    message: request
                response:
                  status: 200 OK
                  statusCode: 200
                  header:
                    Content-Length:
                    - "23"
                    Content-Type:
                    - application/json
                    Date:
                    - Mon, 01 Jan 0001 00:00:00 GMT
                  body:
                    message: request
                elapsed time: 0.000000 sec
                expected "response" but got "request"
                      12 |   expect:
                      13 |     code: 200
                      14 |     body:
                    > 15 |       message: "response"
                                          ^
FAIL
FAIL	scenarios/fail.yaml	0.000s
FAIL
ok  	scenarios/pass.yaml	0.000s

2 tests run: 1 passed, 1 failed, 0 skipped

Failed tests:
	- scenarios/fail.yaml

`, "\n"),
		},
		"create reports": {
			args:        []string{},
			config:      "./testdata/scenarigo-report.yaml",
			expectError: ErrTestFailed.Error(),
			expectOutput: strings.TrimPrefix(`
--- FAIL: scenarios/fail.yaml (0.00s)
    --- FAIL: scenarios/fail.yaml//echo (0.00s)
        --- FAIL: scenarios/fail.yaml//echo/POST_/echo (0.00s)
                request:
                  method: POST
                  url: http://127.0.0.1:12345/echo
                  header:
                    User-Agent:
                    - scenarigo/v1.0.0
                  body:
                    message: request
                response:
                  status: 200 OK
                  statusCode: 200
                  header:
                    Content-Length:
                    - "23"
                    Content-Type:
                    - application/json
                    Date:
                    - Mon, 01 Jan 0001 00:00:00 GMT
                  body:
                    message: request
                elapsed time: 0.000000 sec
                expected "response" but got "request"
                      12 |   expect:
                      13 |     code: 200
                      14 |     body:
                    > 15 |       message: "response"
                                          ^
FAIL
FAIL	scenarios/fail.yaml	0.000s
FAIL
`, "\n"),
			expectReports: []string{
				"./testdata/report.json",
				"./testdata/junit.xml",
			},
		},
		"failed to create reports": {
			args:        []string{},
			config:      "./testdata/scenarigo-report-fail.yaml",
			expectError: "failed to create test reports: failed to write JSON test report",
			expectOutput: strings.TrimPrefix(`
ok  	scenarios/pass.yaml	0.000s
`, "\n"),
		},
		"create reports with CLI flags": {
			args:   []string{"testdata/scenarios/pass.yaml"},
			config: "./testdata/scenarigo-no-report.yaml",
			setupFlags: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				cmd.Flags().StringVar(&reportJSON, "report-json", "", "")
				cmd.Flags().StringVar(&reportJUnit, "report-junit", "", "")
				if err := cmd.Flags().Set("report-json", "./report-cli.json"); err != nil {
					t.Fatal(err)
				}
				if err := cmd.Flags().Set("report-junit", "./junit-cli.xml"); err != nil {
					t.Fatal(err)
				}
			},
			expectOutput: strings.TrimPrefix(`
ok  	scenarios/pass.yaml	0.000s
`, "\n"),
			expectReports: []string{
				"./testdata/report-cli.json",
				"./testdata/junit-cli.xml",
			},
		},
		"CLI flags override config file": {
			args:   []string{},
			config: "./testdata/scenarigo-report.yaml",
			setupFlags: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				cmd.Flags().StringVar(&reportJSON, "report-json", "", "")
				cmd.Flags().StringVar(&reportJUnit, "report-junit", "", "")
				if err := cmd.Flags().Set("report-json", "./report-override.json"); err != nil {
					t.Fatal(err)
				}
				if err := cmd.Flags().Set("report-junit", "./junit-override.xml"); err != nil {
					t.Fatal(err)
				}
			},
			expectOutput: strings.TrimPrefix(`
--- FAIL: scenarios/fail.yaml (0.00s)
    --- FAIL: scenarios/fail.yaml//echo (0.00s)
        --- FAIL: scenarios/fail.yaml//echo/POST_/echo (0.00s)
                request:
                  method: POST
                  url: http://127.0.0.1:12345/echo
                  header:
                    User-Agent:
                    - scenarigo/v1.0.0
                  body:
                    message: request
                response:
                  status: 200 OK
                  statusCode: 200
                  header:
                    Content-Length:
                    - "23"
                    Content-Type:
                    - application/json
                    Date:
                    - Mon, 01 Jan 0001 00:00:00 GMT
                  body:
                    message: request
                elapsed time: 0.000000 sec
                expected "response" but got "request"
                      12 |   expect:
                      13 |     code: 200
                      14 |     body:
                    > 15 |       message: "response"
                                          ^
FAIL
FAIL	scenarios/fail.yaml	0.000s
FAIL
`, "\n"),
			expectError: ErrTestFailed.Error(),
			expectReports: []string{
				"./testdata/report-override.json",
				"./testdata/junit-override.xml",
			},
		},
		"CLI flag for JSON only": {
			args:   []string{"testdata/scenarios/pass.yaml"},
			config: "./testdata/scenarigo-report.yaml",
			setupFlags: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				cmd.Flags().StringVar(&reportJSON, "report-json", "", "")
				cmd.Flags().StringVar(&reportJUnit, "report-junit", "", "")
				if err := cmd.Flags().Set("report-json", "./report-json-only.json"); err != nil {
					t.Fatal(err)
				}
			},
			expectOutput: strings.TrimPrefix(`
ok  	scenarios/pass.yaml	0.000s
`, "\n"),
			expectReports: []string{
				"./testdata/report-json-only.json",
				"./testdata/junit.xml", // junit from config file
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() {
				// Clean up report files after test
				for _, file := range test.expectReports {
					os.Remove(file)
				}
			})

			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			config.ConfigPath = test.config

			// Setup CLI flags if needed
			if test.setupFlags != nil {
				test.setupFlags(t, cmd)
			}

			err := run(cmd, test.args)
			if test.expectError != "" {
				if err == nil {
					t.Fatal("expect error but no error")
				} else if !strings.Contains(err.Error(), test.expectError) {
					t.Fatalf("unexpected error: %s", err)
				}
			}
			if test.expectError == "" && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got, expect := sortTests(t, testutil.ReplaceOutput(buf.String())), test.expectOutput; got != expect {
				dmp := diffmatchpatch.New()
				diffs := dmp.DiffMain(expect, got, false)
				t.Errorf("stdout differs:\n%s", dmp.DiffPrettyText(diffs))
			}
			for _, file := range test.expectReports {
				if _, err := os.Stat(file); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

func sortTests(t *testing.T, s string) string {
	t.Helper()
	tests := []string{}
	lines := strings.Split(s, "\n")
	var trailing string
	var i int
loop:
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "ok"):
			tests = append(tests, line)
			i++
		case strings.HasPrefix(line, "--- FAIL:"):
			failLines := []string{line}
			i++
			var fails int
			for i < len(lines) {
				line := lines[i]
				failLines = append(failLines, line)
				i++
				if strings.HasPrefix(line, "FAIL") {
					fails++
					if fails == 3 {
						tests = append(tests, strings.Join(failLines, "\n"))
						break
					}
				}
			}
		case line == "":
			trailing = strings.Join(lines[i:], "\n")
			break loop
		default:
			t.Fatalf("unknown output %q", line)
		}
	}
	sort.Strings(tests)
	return strings.Join(append(tests, trailing), "\n")
}

func TestSetupTracing(t *testing.T) {
	newCmd := func() (*cobra.Command, *bytes.Buffer) {
		var stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&stderr)
		return cmd, &stderr
	}
	tracingConfig := func(exporter string) *schema.Config {
		return &schema.Config{Telemetry: schema.TelemetryConfig{Tracing: schema.TracingConfig{Exporter: exporter}}}
	}

	t.Run("tracing off", func(t *testing.T) {
		for name, cfg := range map[string]*schema.Config{"nil config": nil, "none": tracingConfig("none"), "empty": tracingConfig("")} {
			cmd, stderr := newCmd()
			opts, shutdown, err := setupTracing(cmd, cfg)
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", name, err)
			}
			if len(opts) != 0 {
				t.Errorf("%s: expected no reporter option, got %d", name, len(opts))
			}
			shutdown()
			if stderr.Len() != 0 {
				t.Errorf("%s: stderr = %q", name, stderr.String())
			}
		}
	})

	t.Run("unknown exporter", func(t *testing.T) {
		cmd, _ := newCmd()
		if _, _, err := setupTracing(cmd, tracingConfig("jaeger")); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("otlp", func(t *testing.T) {
		var mu sync.Mutex
		var exports int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			exports++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)
		for _, key := range []string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"} {
			t.Setenv(key, "") // registers the restore
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)

		cmd, stderr := newCmd()
		opts, shutdown, err := setupTracing(cmd, tracingConfig(schema.TracingExporterOTLP))
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if len(opts) != 1 {
			t.Fatalf("expected the tracer provider option, got %d options", len(opts))
		}
		reporter.Run(func(r reporter.Reporter) {
			r.Run("scenario", func(r reporter.Reporter) {})
		}, opts...)
		shutdown()

		mu.Lock()
		defer mu.Unlock()
		if exports == 0 {
			t.Error("shutdown must flush the spans to the collector")
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q", stderr.String())
		}
	})
}
