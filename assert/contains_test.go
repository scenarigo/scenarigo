package assert

import (
	"testing"
)

func TestContains(t *testing.T) {
	tests := map[string]struct {
		in          any
		contains    int
		expectError bool
	}{
		"not array or slice": {
			in:          0,
			contains:    0,
			expectError: true,
		},
		"empty": {
			in:          []int{},
			contains:    0,
			expectError: true,
		},
		"contains": {
			in:       []int{0, 1},
			contains: 1,
		},
		"not contains": {
			in:          []int{0, 1},
			contains:    2,
			expectError: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assertion := Contains(Equal(test.contains))
			err := assertion.Assert(test.in)
			if !test.expectError && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if test.expectError && err == nil {
				t.Fatal("expected error but got no error")
			}
		})
	}
}

func TestNotContains(t *testing.T) {
	tests := map[string]struct {
		in          any
		notContains int
		expectError bool
	}{
		"not array or slice": {
			in:          0,
			notContains: 0,
			expectError: true,
		},
		"empty": {
			in:          []int{},
			notContains: 0,
		},
		"contains": {
			in:          []int{0, 1},
			notContains: 1,
			expectError: true,
		},
		"not contains": {
			in:          []int{0, 1},
			notContains: 2,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assertion := NotContains(Equal(test.notContains))
			err := assertion.Assert(test.in)
			if !test.expectError && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if test.expectError && err == nil {
				t.Fatal("expected error but got no error")
			}
		})
	}
}
