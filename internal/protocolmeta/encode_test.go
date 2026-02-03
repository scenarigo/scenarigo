package protocolmeta

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEncodeHTTPValue(t *testing.T) {
	tests := map[string]struct {
		input  string
		expect string
	}{
		"ascii": {
			input:  "test.yaml",
			expect: "test.yaml",
		},
		"with spaces": {
			input:  "test scenario",
			expect: "test%20scenario",
		},
		"with slashes": {
			input:  "testdata/scenarios/test.yaml",
			expect: "testdata%2Fscenarios%2Ftest.yaml",
		},
		"japanese": {
			input:  "テスト/シナリオ.yaml",
			expect: "%E3%83%86%E3%82%B9%E3%83%88%2F%E3%82%B7%E3%83%8A%E3%83%AA%E3%82%AA.yaml",
		},
		"empty": {
			input:  "",
			expect: "",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(test.expect, EncodeHTTPValue(test.input)); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}
