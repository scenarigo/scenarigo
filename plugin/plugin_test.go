//go:build !race

package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get current directory: %s", err)
		}

		tests := map[string]struct {
			path string
		}{
			"absolute path": {
				path: "../test/e2e/testdata/gen/plugins/simple.so",
			},
			"relative path": {
				path: filepath.Join(wd, "../test/e2e/testdata/gen/plugins/simple.so"),
			},
		}

		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				resetCache()

				// open plugin
				p, err := Open(test.path)
				if err != nil {
					t.Fatalf("failed to open plugin: %s", err)
				}

				// use plugin
				v, err := p.Lookup("Function")
				if err != nil {
					t.Fatalf("failed to lookup: %s", err)
				}
				f, ok := v.(func() string)
				if !ok {
					t.Fatalf("expect func() but got %T", v)
				}
				if got, expect := f(), "function"; got != expect {
					t.Fatalf("expect %s but got %s", expect, got)
				}

				// get from cache
				pp, err := Open(filepath.Clean(filepath.Join(wd, "../test/e2e/testdata/gen/plugins/simple.so")))
				if err != nil {
					t.Fatalf("failed to open plugin: %s", err)
				}
				if pp != p {
					t.Fatalf("failed to get from cache: got->%p cache->%p", pp, p)
				}
			})
		}
	})
}

func resetCache() {
	m.Lock()
	defer m.Unlock()
	cache = map[string]Plugin{}
}
