package reporter

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/zoncoen/scenarigo/internal/ptr"
)

func TestLogRecorder(t *testing.T) {
	tests := map[string]struct {
		f           func(r *logRecorder)
		expect      *logRecorder
		expectInfo  []string
		expectError []string
		expectSkip  *string
	}{
		"basic": {
			f: func(r *logRecorder) {
				r.log("info")
				r.error("error")
				r.skip("skip")
			},
			expect: &logRecorder{
				strs:      []string{"info", "error", "skip"},
				infoIdxs:  []int{0},
				errorIdxs: []int{1},
				skipIdx:   ptr.To(2),
			},
			expectInfo:  []string{"info"},
			expectError: []string{"error"},
			expectSkip:  ptr.To("skip"),
		},
		"setReplacer first": {
			f: func(r *logRecorder) {
				r.setReplacer(replacer("sec", "XXX"))
				r.log("info sec")
				r.error("error sec")
				r.skip("skip sec")
			},
			expect: &logRecorder{
				strs:      []string{"info XXX", "error XXX", "skip XXX"},
				infoIdxs:  []int{0},
				errorIdxs: []int{1},
				skipIdx:   ptr.To(2),
			},
			expectInfo:  []string{"info XXX"},
			expectError: []string{"error XXX"},
			expectSkip:  ptr.To("skip XXX"),
		},
		"setReplacer last": {
			f: func(r *logRecorder) {
				r.log("info sec")
				r.error("error sec")
				r.skip("skip sec")
				r.setReplacer(replacer("sec", "XXX"))
			},
			expect: &logRecorder{
				strs:      []string{"info XXX", "error XXX", "skip XXX"},
				infoIdxs:  []int{0},
				errorIdxs: []int{1},
				skipIdx:   ptr.To(2),
			},
			expectInfo:  []string{"info XXX"},
			expectError: []string{"error XXX"},
			expectSkip:  ptr.To("skip XXX"),
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			r := &logRecorder{}
			test.f(r)
			opts := []cmp.Option{
				cmp.AllowUnexported(logRecorder{}),
				cmp.FilterPath(func(p cmp.Path) bool {
					if p.Last().String() == ".m" {
						return true
					}
					if p.Last().String() == ".replacer" {
						return true
					}
					return false
				}, cmp.Ignore()),
			}
			if diff := cmp.Diff(test.expect, r, opts...); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.expect.strs, r.all()); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.expectInfo, r.infoLogs()); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.expectError, r.errorLogs()); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.expectSkip, r.skipLog()); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func replacer(old, new string) LogReplacer {
	return logReplacer(func(s string) string {
		return strings.ReplaceAll(s, old, new)
	})
}

type logReplacer func(string) string

func (r logReplacer) ReplaceAll(s string) string {
	return r(s)
}

func TestLogRecorder_Append(t *testing.T) {
	s := &logRecorder{}
	s.log("INFO")
	s.error("ERROR")
	s.skip("SKIP")

	opts := []cmp.Option{
		cmp.AllowUnexported(logRecorder{}),
		cmp.FilterPath(func(p cmp.Path) bool {
			return p.Last().String() == ".m"
		}, cmp.Ignore()),
	}

	t.Run("skipIdx is nil", func(t *testing.T) {
		r := &logRecorder{}
		r.log("info")
		r.error("error")
		r.append(s)

		skipIdx := 4
		skipLog := "SKIP"
		if diff := cmp.Diff(&logRecorder{
			strs:      []string{"info", "error", "INFO", "ERROR", "SKIP"},
			infoIdxs:  []int{0, 2},
			errorIdxs: []int{1, 3},
			skipIdx:   &skipIdx,
		}, r, opts...); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]string{"info", "error", "INFO", "ERROR", "SKIP"}, r.all()); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]string{"info", "INFO"}, r.infoLogs()); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]string{"error", "ERROR"}, r.errorLogs()); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(&skipLog, r.skipLog()); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	})
	t.Run("skipIdx is not nil", func(t *testing.T) {
		r := &logRecorder{}
		r.log("info")
		r.error("error")
		r.skip("skip")
		r.append(s)

		skipIdx := 2
		skipLog := "skip"
		if diff := cmp.Diff(&logRecorder{
			strs:      []string{"info", "error", "skip", "INFO", "ERROR", "SKIP"},
			infoIdxs:  []int{0, 3, 5},
			errorIdxs: []int{1, 4},
			skipIdx:   &skipIdx,
		}, r, opts...); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]string{"info", "error", "skip", "INFO", "ERROR", "SKIP"}, r.all()); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]string{"info", "INFO", "SKIP"}, r.infoLogs()); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]string{"error", "ERROR"}, r.errorLogs()); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(&skipLog, r.skipLog()); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	})
}
