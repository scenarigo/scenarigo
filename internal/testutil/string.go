package testutil

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scenarigo/scenarigo/internal/protocolmeta"
	"github.com/scenarigo/scenarigo/version"
)

var (
	dddPattern         = regexp.MustCompile(`\d\.\d\ds`)
	ddddPattern        = regexp.MustCompile(`\d\.\d\d\ds`)
	elapsedTimePattern = regexp.MustCompile(`elapsed time(:)? .+`)
	ipv4AddrPattern    = regexp.MustCompile(`127.0.0.1:\d+`)
	ipv6AddrPattern    = regexp.MustCompile(`\[::\]:\d+`)
	userAgentPattern   = regexp.MustCompile(fmt.Sprintf(`- scenarigo/%s`, version.String()))
	dateHeaderPattern  = regexp.MustCompile(`Date:\n\s*- (.+)`)
)

// ReplaceOption is an option for ReplaceOutput.
type ReplaceOption func(*replaceConfig)

type replaceConfig struct {
	keepScenarigoHeaders bool
}

// KeepScenarigoHeaders keeps scenarigo headers in the output.
func KeepScenarigoHeaders() ReplaceOption {
	return func(c *replaceConfig) {
		c.keepScenarigoHeaders = true
	}
}

// ReplaceOutput replaces result output.
func ReplaceOutput(s string, opts ...ReplaceOption) string {
	cfg := &replaceConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	funcs := []func(string) string{
		ResetDuration,
		ReplaceAddr,
		ReplaceUserAgent,
		ReplaceDateHeader,
		ReplaceFilepath,
		ReplacePluginOpen,
	}
	if !cfg.keepScenarigoHeaders {
		funcs = append(funcs, RemoveScenarigoHeaders)
	}

	for _, f := range funcs {
		s = f(s)
	}
	return s
}

// ResetDuration resets durations from result output.
func ResetDuration(s string) string {
	s = dddPattern.ReplaceAllString(s, "0.00s")
	s = ddddPattern.ReplaceAllString(s, "0.000s")
	return elapsedTimePattern.ReplaceAllString(s, "elapsed time: 0.000000 sec")
}

// ReplaceAddr replaces addresses on result output.
func ReplaceAddr(s string) string {
	s = ipv4AddrPattern.ReplaceAllString(s, "127.0.0.1:12345")
	return ipv6AddrPattern.ReplaceAllString(s, "[::]:12345")
}

// ReplaceUserAgent replaces User-Agent header on result output.
func ReplaceUserAgent(s string) string {
	return userAgentPattern.ReplaceAllString(s, "- scenarigo/v1.0.0")
}

// ReplaceDateHeader replaces Date header on result output.
func ReplaceDateHeader(s string) string {
	found := dateHeaderPattern.FindAllStringSubmatch(s, -1)
	for _, subs := range found {
		if len(subs) > 1 {
			s = strings.ReplaceAll(s, subs[1], "Mon, 01 Jan 0001 00:00:00 GMT")
		}
	}
	return s
}

// ReplaceFilepath replaces filepaths.
func ReplaceFilepath(s string) string {
	wd, err := os.Getwd()
	if err != nil {
		return s
	}
	root := wd
	parts := strings.Split(filepath.ToSlash(wd), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "scenarigo" {
			root = filepath.FromSlash(strings.Join(parts[:i+1], "/"))
			break
		}
	}
	result := strings.ReplaceAll(s, root, filepath.FromSlash("/go/src/github.com/scenarigo/scenarigo"))

	// Additional pattern-based replacement for any scenarigo path that wasn't caught
	// This uses regex to find any path ending with "scenarigo" and normalize it
	// Only match actual file paths (starting with / or containing filesystem separators)
	scenarigoPathRe := regexp.MustCompile(`(/[^/\s]*)+/scenarigo\b`)
	result = scenarigoPathRe.ReplaceAllString(result, "/go/src/github.com/scenarigo/scenarigo")

	return result
}

// ReplacePluginOpen normalizes plugin.Open error messages to open error messages.
func ReplacePluginOpen(s string) string {
	// Only replace plugin.Open errors for WASM files, not .so files
	wasmPluginOpenPattern := regexp.MustCompile(`plugin\.Open\("([^"]*\.wasm)"\): realpath failed`)
	return wasmPluginOpenPattern.ReplaceAllString(s, "open ${1}: no such file or directory")
}

var scenarigoHeaderKeys = map[string]struct{}{
	http.CanonicalHeaderKey(protocolmeta.ScenarigoScenarioFilepathKey) + ":": {},
	http.CanonicalHeaderKey(protocolmeta.ScenarigoScenarioTitleKey) + ":":    {},
	http.CanonicalHeaderKey(protocolmeta.ScenarigoStepFullNameKey) + ":":     {},
}

// RemoveScenarigoHeaders drops scenarigo-specific request headers from output.
func RemoveScenarigoHeaders(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	skipValue := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if skipValue {
			if strings.HasPrefix(trimmed, "- ") {
				skipValue = false
				continue
			}
			skipValue = false
		}
		if _, ok := scenarigoHeaderKeys[trimmed]; ok {
			skipValue = true
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
