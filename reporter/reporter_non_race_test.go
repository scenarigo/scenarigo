//go:build !race

package reporter

import (
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// TODO: This test case causes a data race in Go1.21.
// https://github.com/golang/go/commit/213495a4a67c318a1fab6e76093e6690c2141c0e#diff-fca95769950854d54b8704162c0b104c189ed1096633227c1cbbf0c49739ccde
func TestRun_NilPanic(t *testing.T) {
	tests := map[string]struct {
		maxParallel            int
		fs                     []func(r Reporter)
		expect                 result
		expectSerialDuration   time.Duration
		expectParallelDuration time.Duration
	}{
		"child panic(nil)": {
			fs: []func(r Reporter){
				func(r Reporter) {},
				func(r Reporter) {
					panic(nil)
				},
			},
			expect: result{
				Failed: true,
				Children: []result{
					{},
					{
						Failed: true,
						Logs:   []string{"panic called with nil argument"},
					},
				},
			},
		},
		"child runtime.Goexit()": {
			fs: []func(r Reporter){
				func(r Reporter) {},
				func(r Reporter) {
					runtime.Goexit()
				},
			},
			expect: result{
				Failed: true,
				Children: []result{
					{},
					{
						Failed: true,
						Logs:   []string{"test executed panic(nil) or runtime.Goexit"},
					},
				},
			},
		},
	}
	for name, test := range tests {
		opts := []Option{}
		if test.maxParallel != 0 {
			opts = append(opts, WithMaxParallel(test.maxParallel))
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Run("serial", func(t *testing.T) {
				t.Parallel()
				start := time.Now()
				r := run(func(r Reporter) {
					for i, f := range test.fs {
						r.Run(strconv.Itoa(i), func(r Reporter) {
							f(r)
						})
					}
				}, opts...)
				duration := time.Since(start)
				if diff := cmp.Diff(test.expect, ignoreStackTrace(collectResult(r))); diff != "" {
					t.Errorf("result mismatch (-want +got):\n%s", diff)
				}
				if expect, got := test.expectSerialDuration, duration.Truncate(5*durationTestUnit); got != expect {
					t.Errorf("expected %s but got %s", expect, got)
				}
				if expect, got := test.expectSerialDuration, r.getDuration().Truncate(5*durationTestUnit); got != expect {
					t.Errorf("expected %s but got %s", expect, got)
				}
			})
			t.Run("parallel", func(t *testing.T) {
				t.Parallel()
				start := time.Now()
				r := run(func(r Reporter) {
					for i, f := range test.fs {
						r.Run(strconv.Itoa(i), func(r Reporter) {
							r.Parallel()
							f(r)
						})
					}
				}, opts...)
				duration := time.Since(start)
				if diff := cmp.Diff(test.expect, ignoreStackTrace(collectResult(r))); diff != "" {
					t.Errorf("result mismatch (-want +got):\n%s", diff)
				}
				if expect, got := test.expectParallelDuration, duration.Truncate(5*durationTestUnit); got != expect {
					t.Errorf("expected %s but got %s", expect, got)
				}
				if expect, got := test.expectParallelDuration, r.getDuration().Truncate(5*durationTestUnit); got != expect {
					t.Errorf("expected %s but got %s", expect, got)
				}
			})
		})
	}
}
