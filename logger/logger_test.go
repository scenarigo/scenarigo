package logger

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestLogger(t *testing.T) {
	tests := map[string]struct {
		level  LogLevel
		f      func(l Logger)
		expect string
	}{
		"info": {
			level: LogLevelAll,
			f: func(l Logger) {
				l.Info("info msg", "count", 1)
				l.Error(errors.New("omg"), "error msg", "count", 2)
			},
			expect: strings.TrimPrefix(`
[INFO] "info msg" "count"="1"
[ERROR] "error msg" "error"="omg" "count"="2"
`, "\n"),
		},
		"error": {
			level: LogLevelError,
			f: func(l Logger) {
				l.Info("info msg", "count", 1)
				l.Error(errors.New("omg"), "error msg", "count", 2)
			},
			expect: strings.TrimPrefix(`
[ERROR] "error msg" "error"="omg" "count"="2"
`, "\n"),
		},
		"no value": {
			level: LogLevelAll,
			f: func(l Logger) {
				l.Info("info msg", "count")
			},
			expect: strings.TrimPrefix(`
[INFO] "info msg" "count"="<no-value>"
`, "\n"),
		},
		"none": {
			level: LogLevelNone,
			f: func(l Logger) {
				l.Info("info msg", "count", 1)
				l.Error(errors.New("omg"), "error msg", "count", 2)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			l := NewLogger(log.New(&b, "", 0), test.level)
			test.f(l)
			if got, expect := b.String(), test.expect; got != expect {
				t.Errorf(`
=== got ===
%s
=== expect ===
%s
`, got, expect)
			}
		})
	}
}
