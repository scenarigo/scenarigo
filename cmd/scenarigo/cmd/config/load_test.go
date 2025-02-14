package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tempDir := t.TempDir()
	f, err := os.Create(filepath.Join(tempDir, DefaultConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(defaultConfig); err != nil {
		t.Fatalf("failed to create config file for testing: %s", err)
	}

	tests := map[string]struct {
		filename string
		root     string
		cd       string
		found    bool
		fail     bool
	}{
		"default (not found)": {},
		"default (found)": {
			cd:    tempDir,
			found: true,
		},
		"specify file": {
			filename: filepath.Join(tempDir, DefaultConfigFileName),
			found:    true,
		},
		"specify file with root": {
			filename: filepath.Join(tempDir, DefaultConfigFileName),
			root:     tempDir,
			found:    true,
		},
		"stdin": {
			filename: "-",
			found:    true,
		},
		"stdin with root": {
			filename: "-",
			root:     tempDir,
			found:    true,
		},
		"specify file (not found)": {
			filename: "testdata/not-found.yaml",
			fail:     true,
		},
		"invalid config": {
			filename: "testdata/invalid.yaml",
			fail:     true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.cd != "" {
				wd, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := os.Chdir(wd); err != nil {
						t.Fatal(err)
					}
				})
				if err := os.Chdir(test.cd); err != nil {
					t.Fatal(err)
				}
			}

			ConfigPath = test.filename
			if ConfigPath == "-" {
				stdin := os.Stdin
				t.Cleanup(func() {
					os.Stdin = stdin
				})
				in, err := os.Open("default.scenarigo.yaml")
				if err != nil {
					t.Fatal(err)
				}
				defer in.Close()
				os.Stdin = in
			}
			Root = test.root

			cfg, err := Load()
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
