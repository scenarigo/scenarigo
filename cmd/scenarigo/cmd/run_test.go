package cmd

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/scenarigo/scenarigo/internal/testutil"
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
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			for _, file := range test.expectReports {
				if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed to remove: %s", err)
				}
			}

			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			config.ConfigPath = test.config
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
			if got, expect := testutil.ReplaceOutput(buf.String()), test.expectOutput; got != expect {
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
