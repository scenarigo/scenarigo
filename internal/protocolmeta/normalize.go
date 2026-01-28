package protocolmeta

import (
	"os"
	"path/filepath"
)

// NormalizeScenarioFilepath returns a stable, relative filepath when possible.
func NormalizeScenarioFilepath(path string) string {
	if path == "" || !filepath.IsAbs(path) {
		return path
	}
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, path)
	if err != nil {
		return path
	}
	return rel
}
