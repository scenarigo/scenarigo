package protocolmeta

import (
	"os"
	"path/filepath"
)

// NormalizeScenarioFilepath returns a stable, relative filepath when possible.
// It keeps the working-directory basis and the OS separator of the legacy
// scenarigo-* headers; NormalizeFilepath is the rootDir-based variant used for
// baggage and span attributes.
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

// NormalizeFilepath returns path relative to rootDir with "/" separators.
// rootDir is the directory of the configuration file; an empty rootDir falls
// back to the working directory (library use without a config). When the
// relative path cannot be computed (another volume on Windows), the absolute
// path is returned with "/" separators so that the value is still a structural key.
func NormalizeFilepath(rootDir, path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	if rootDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return filepath.ToSlash(abs)
		}
		rootDir = wd
	}
	rel, err := filepath.Rel(rootDir, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}
