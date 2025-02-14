package assert

import (
	"testing"
)

func TestLength(t *testing.T) {
	tests := map[string]struct {
		expect interface{}
		ok     interface{}
		ng     interface{}
	}{
		"string": {
			expect: 1,
			ok:     "あ",
			ng:     "aa",
		},
		"array": {
			expect: 1,
			ok:     [1]int{1},
			ng:     [2]int{1, 2},
		},
		"slice": {
			expect: 1,
			ok:     []int{1},
			ng:     []int{},
		},
		"map": {
			expect: 1,
			ok: map[int]bool{
				1: true,
			},
			ng: map[int]bool{},
		},
		"Assertion": {
			expect: Greater(0),
			ok:     []int{1},
			ng:     []int{},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assertion := Length(test.expect)
			t.Run("ok", func(t *testing.T) {
				if err := assertion.Assert(test.ok); err != nil {
					t.Errorf("unexpected error: %s", err)
				}
			})
			t.Run("ng", func(t *testing.T) {
				if err := assertion.Assert(test.ng); err == nil {
					t.Error("no error")
				}
			})
		})
	}
}

func TestLength_Error(t *testing.T) {
	t.Run("invalid expect", func(t *testing.T) {
		if err := Length("0").Assert([]int{}); err == nil {
			t.Error("no error")
		} else if got, expect := err.Error(), `invalid expected length "0"`; got != expect {
			t.Errorf("expected %q but got %q", expect, err)
		}
	})
	t.Run("failed to get length", func(t *testing.T) {
		if err := Length(0).Assert(0); err == nil {
			t.Error("no error")
		} else if got, expect := err.Error(), "can't get the length of int"; got != expect {
			t.Errorf("expected %q but got %q", expect, err)
		}
	})
}
