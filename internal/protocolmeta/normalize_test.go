package protocolmeta

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNormalizeScenarioFilepath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %s", err)
	}

	tests := map[string]struct {
		input  string
		expect string
	}{
		"empty": {
			input:  "",
			expect: "",
		},
		"relative path": {
			input:  "testdata/test.yaml",
			expect: "testdata/test.yaml",
		},
		"absolute path in working directory": {
			input:  filepath.Join(wd, "testdata", "test.yaml"),
			expect: filepath.Join("testdata", "test.yaml"),
		},
		"absolute path in subdirectory": {
			input:  filepath.Join(wd, "scenarios", "sub", "test.yaml"),
			expect: filepath.Join("scenarios", "sub", "test.yaml"),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(test.expect, NormalizeScenarioFilepath(test.input)); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}
