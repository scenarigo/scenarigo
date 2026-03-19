package filepathutil

import "testing"

func TestFrom(t *testing.T) {
	tests := map[string]struct {
		base   string
		path   string
		expect string
	}{
		"relative": {
			base:   "/path/to/base",
			path:   "./scenarigo.yaml",
			expect: "/path/to/base/scenarigo.yaml",
		},
		"absolute": {
			base:   "/path/to/base",
			path:   "/etc/scenarigo/scenarigo.yaml",
			expect: "/etc/scenarigo/scenarigo.yaml",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got, expect := From(test.base, test.path), test.expect; got != expect {
				t.Errorf("expect %q but got %q", expect, got)
			}
		})
	}
}
