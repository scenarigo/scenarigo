package assert

import (
	"testing"
)

func TestAnd(t *testing.T) {
	if err := And().Assert(""); err == nil {
		t.Fatal("empty assertion list should be an error")
	}

	tests := map[string]struct {
		assertions  []Assertion
		ok          any
		ng          any
		expectError string
	}{
		"empty": {
			assertions:  []Assertion{},
			ng:          "1",
			expectError: "empty assertion list",
		},
		"one": {
			assertions: []Assertion{
				Equal("one"),
			},
			ok:          "one",
			ng:          "1",
			expectError: `[0]: expected "one" but got "1"`,
		},
		"two": {
			assertions: []Assertion{
				Equal("one"),
				NotZero(),
			},
			ok:          "one",
			ng:          "1",
			expectError: `[0]: expected "one" but got "1"`,
		},
		"multi error": {
			assertions: []Assertion{
				Equal("one"),
				Equal("un"),
				NotZero(),
			},
			ng: "1",
			expectError: `2 errors occurred: [0]: expected "one" but got "1"
[1]: expected "un" but got "1"`,
		},
	}
	for _, test := range tests {
		and := And(test.assertions...)
		if test.ok != nil {
			if err := and.Assert(test.ok); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
		}
		if err := and.Assert(test.ng); err == nil {
			t.Error("expect error but no error")
		} else if got, expect := err.Error(), test.expectError; got != expect {
			t.Errorf("expect error %q but got %q", expect, got)
		}
	}
}

func TestOr(t *testing.T) {
	if err := Or().Assert(""); err == nil {
		t.Fatal("empty assertion list should be an error")
	}

	tests := map[string]struct {
		assertions  []Assertion
		ok          any
		ng          any
		expectError string
	}{
		"empty": {
			assertions:  []Assertion{},
			ng:          "2",
			expectError: "empty assertion list",
		},
		"one": {
			assertions: []Assertion{
				Equal("two"),
			},
			ok:          "two",
			ng:          "2",
			expectError: `[0]: all assertions failed: expected "two" but got "2"`,
		},
		"two": {
			assertions: []Assertion{
				Equal("one"),
				Equal("two"),
			},
			ok: "two",
			ng: "2",
			expectError: `2 errors occurred: [0]: all assertions failed: expected "one" but got "2"
[1]: all assertions failed: expected "two" but got "2"`,
		},
	}
	for _, test := range tests {
		or := Or(test.assertions...)
		if test.ok != nil {
			if err := or.Assert(test.ok); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
		}
		if err := or.Assert(test.ng); err == nil {
			t.Error("expect error but no error")
		} else if got, expect := err.Error(), test.expectError; got != expect {
			t.Errorf("expect error %q but got %q", expect, got)
		}
	}
}
