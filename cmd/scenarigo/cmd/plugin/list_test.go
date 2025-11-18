package plugin

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/cobra"
)

func TestList(t *testing.T) {
	tests := map[string]struct {
		wd          string
		config      string
		expect      string
		expectError bool
	}{
		"valid config": {
			config: "testdata/config/valid.yaml",
			expect: strings.TrimPrefix(`
testdata/config/gen/local.so
testdata/config/gen/remote.so
`, "\n"),
		},
		"valid config (change dir)": {
			wd:     "testdata",
			config: "config/valid.yaml",
			expect: strings.TrimPrefix(`
config/gen/local.so
config/gen/remote.so
`, "\n"),
		},
		"invalid config": {
			wd:          "testdata",
			config:      "config/invalid.yaml",
			expectError: true,
		},
		"default config not found": {
			wd:          "testdata",
			expectError: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			wd, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get current directory: %s", err)
			}
			t.Chdir(filepath.Join(wd, test.wd))

			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			config.ConfigPath = test.config

			if err := list(cmd, []string{}); err != nil {
				if test.expectError {
					return
				}
				t.Fatal(err)
			} else if test.expectError {
				t.Fatal("no error")
			}
			if got, expect := buf.String(), test.expect; got != expect {
				dmp := diffmatchpatch.New()
				diffs := dmp.DiffMain(expect, got, false)
				t.Errorf("stdout differs:\n%s", dmp.DiffPrettyText(diffs))
			}
		})
	}
}
