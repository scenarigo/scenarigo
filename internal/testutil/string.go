package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	ansiColorPattern   = regexp.MustCompile(`\[[0-9;]*m`)
	timeoutPattern     = regexp.MustCompile(`\s*step hasn't finished in .+ despite the context canceled\n?`)
)

// ReplaceOutput replaces result output.
func ReplaceOutput(s string) string {
	for _, f := range []func(string) string{
		ResetDuration,
		ReplaceAddr,
		ReplaceUserAgent,
		ReplaceDateHeader,
		ReplaceFilepath,
		ReplacePluginOpen,
		RemoveAnsiColors,
		RemoveTimeoutMessages,
	} {
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

// RemoveAnsiColors removes ANSI color codes from the output.
func RemoveAnsiColors(s string) string {
	return ansiColorPattern.ReplaceAllString(s, "")
}

// RemoveTimeoutMessages removes timeout-specific messages that may differ between plugin types.
func RemoveTimeoutMessages(s string) string {
	return timeoutPattern.ReplaceAllString(s, "\n")
}
