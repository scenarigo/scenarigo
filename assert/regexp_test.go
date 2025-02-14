package assert

import (
	"testing"
)

func TestRegexp(t *testing.T) {
	type myString string
	tests := map[string]struct {
		expr string
		ok   interface{}
		ng   interface{}
	}{
		"simple": {
			expr: "a",
			ok:   "abc",
			ng:   "ABC",
		},
		"with flag (ignore case)": {
			expr: "(?i)a",
			ok:   "ABC",
			ng:   "def",
		},
		"[]byte": {
			expr: "a",
			ok:   []byte("abc"),
			ng:   []byte("ABC"),
		},
		"string (type conversion)": {
			expr: "a",
			ok:   myString("abc"),
			ng:   myString("ABC"),
		},
		"must be a string": {
			expr: "true",
			ok:   "true",
			ng:   true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assertion := Regexp(tc.expr)
			if err := assertion.Assert(tc.ok); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if err := assertion.Assert(tc.ng); err == nil {
				t.Errorf("expected error but no error")
			}
		})
	}

	t.Run("failed to compile", func(t *testing.T) {
		// invalid flag "a"
		assertion := Regexp("(?a)")
		if err := assertion.Assert("a"); err == nil {
			t.Errorf("expected error but no error")
		}
	})
}
