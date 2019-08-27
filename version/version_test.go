package version

import "testing"

func TestString(t *testing.T) {
	tests := map[string]struct {
		version  string
		revision string
		expect   string
	}{
		"with revision": {
			version:  "1.0.0",
			revision: "beta",
			expect:   "1.0.0-beta",
		},
		"without revision": {
			version:  "1.0.0",
			revision: "",
			expect:   "1.0.0",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			version = test.version
			revision = test.revision
			if got := String(); got != test.expect {
				t.Errorf("expect %s but got %s", test.expect, got)
			}
		})
	}
}
