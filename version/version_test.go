package version

import (
	"runtime/debug"
	"testing"
)

func TestString(t *testing.T) {
	tests := map[string]struct {
		version  string
		revision string
		info     *debug.BuildInfo
		ok       bool
		expect   string
	}{
		"with revision": {
			version:  "1.0.0",
			revision: "beta",
			expect:   "v1.0.0-beta",
		},
		"without revision": {
			version:  "1.0.0",
			revision: "",
			expect:   "v1.0.0",
		},
		"ok but no sum": {
			version:  "1.0.0",
			revision: "beta",
			info: &debug.BuildInfo{
				Main: debug.Module{
					Version: "(devel)",
				},
			},
			ok:     true,
			expect: "v1.0.0-beta",
		},
		"from build info": {
			version:  "1.0.0",
			revision: "beta",
			info: &debug.BuildInfo{
				Main: debug.Module{
					Version: "v2.0.0",
					Sum:     "XXX",
				},
			},
			ok:     true,
			expect: "v2.0.0",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			version = test.version
			revision = test.revision
			info = test.info
			ok = test.ok
			if got := String(); got != test.expect {
				t.Errorf("expect %s but got %s", test.expect, got)
			}
		})
	}
}
