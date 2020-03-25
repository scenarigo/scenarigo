package cmd

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
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
