package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
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

	os.Setenv("TEST_ADDR", srv.URL)
	defer os.Unsetenv("TEST_ADDR")

	tests := map[string]struct {
		file        string
		expectError bool
	}{
		"pass": {
			file:        "testdata/pass.yaml",
			expectError: false,
		},
		"fail": {
			file:        "testdata/fail.yaml",
			expectError: true,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			err := run(nil, []string{test.file})
			if test.expectError && err == nil {
				t.Fatal("expect error but no error")
			}
			if !test.expectError && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	tests := map[string]struct {
		filename string
		cd       string
		found    bool
		fail     bool
	}{
		"default (not found)": {},
		"default (found)": {
			cd:    "./testdata",
			found: true,
		},
		"specify file": {
			filename: "./testdata/scenarigo.yaml",
			found:    true,
		},
		"specify file (not found)": {
			filename: "./testdata/.invalid.yaml",
			fail:     true,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			if test.cd != "" {
				wd, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(test.cd); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.Chdir(wd); err != nil {
						t.Fatal(err)
					}
				}()
			}
			cfg, err := loadConfig(test.filename)
			if test.fail && err == nil {
				t.Fatal("no error")
			}
			if !test.fail && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if test.found && cfg == nil {
				t.Error("config not found")
			}
			if !test.found && cfg != nil {
				t.Error("unknown config")
			}
		})
	}
}
