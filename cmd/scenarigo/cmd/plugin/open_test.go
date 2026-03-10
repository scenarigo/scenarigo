//go:build !race

package plugin

import (
	"plugin"
	"testing"
)

func openPlugin(t *testing.T, p string) {
	t.Helper()
	if _, err := plugin.Open(p); err != nil {
		t.Fatalf("failed to open plugin: %s", err)
	}
}
