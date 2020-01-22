package testutil

import "regexp"

var (
	dddPattern  = regexp.MustCompile(`\d\.\d\ds`)
	ddddPattern = regexp.MustCompile(`\d\.\d\d\ds`)
)

// ResetDuration resets durations from result output.
func ResetDuration(s string) string {
	s = dddPattern.ReplaceAllString(s, "0.00s")
	return ddddPattern.ReplaceAllString(s, "0.000s")
}
