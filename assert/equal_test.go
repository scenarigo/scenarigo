package assert

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/scenarigo/scenarigo/errors"
)

func TestEqual(t *testing.T) {
	s := "string"
	type myString string
	tests := map[string]struct {
		expected  interface{}
		customEqs []Equaler
		ok        interface{}
		ng        interface{}
	}{
		"nil": {
			expected: nil,
			ok:       nil,
			ng:       &s,
		},
		"nil (got typed nil)": {
			expected: nil,
			ok:       (*string)(nil),
			ng:       &s,
		},
		"nil (expect typed nil)": {
			expected: (*string)(nil),
			ok:       nil,
			ng:       &s,
		},
		"integer": {
			expected: 1,
			ok:       1,
			ng:       2,
		},
		"integer (type conversion)": {
			expected: 1,
			ok:       uint64(1),
			ng:       uint64(2),
		},
		"string": {
			expected: "test",
			ok:       "test",
			ng:       "develop",
		},
		"string (type conversion)": {
			expected: "test",
			ok:       myString("test"),
			ng:       myString("develop"),
		},
		"custom equalers": {
			expected: "test",
			customEqs: []Equaler{
				EqualerFunc(func(x, y any) (bool, error) {
					return false, nil
				}),
				EqualerFunc(func(x, y any) (bool, error) {
					s, ok := y.(string)
					if !ok {
						return false, nil
					}
					return s == "dev", nil
				}),
			},
			ok: "dev",
			ng: "prod",
		},
		"json.Number (string)": {
			expected: "100",
			ok:       json.Number("100"),
			ng:       json.Number("0.01"),
		},
		"json.Number (int)": {
			expected: 100,
			ok:       json.Number("100"),
			ng:       json.Number("0.01"),
		},
		"json.Number (float)": {
			expected: 0.01,
			ok:       json.Number("0.01"),
			ng:       json.Number("100"),
		},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			assertion := Equal(tc.expected, tc.customEqs...)
			if err := assertion.Assert(tc.ok); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if err := assertion.Assert(tc.ng); err == nil {
				t.Errorf("expected error but no error")
			}
		})
	}
}

func TestCustomEqualer(t *testing.T) {
	RegisterCustomEqualer(EqualerFunc(func(a, b interface{}) (bool, error) {
		v, ok := a.(int)
		if !ok {
			return false, nil
		}
		if v == 100 {
			// 1 always succeeds
			return true, nil
		}
		return true, errors.New("custom equaler 1")
	}))
	RegisterCustomEqualer(EqualerFunc(func(a, b interface{}) (bool, error) {
		v, ok := a.(bool)
		if !ok {
			return false, nil
		}
		if v {
			// true always succeeds
			return true, nil
		}
		return true, errors.New("custom equaler 2")
	}))
	defer func() {
		m.Lock()
		defer m.Unlock()
		equalers = nil
	}()

	tests := map[string]struct {
		expected interface{}
		got      interface{}
		error    error
	}{
		"equal": {
			expected: 0,
			got:      0,
			error:    nil,
		},
		"not equal but passed by first custom equaler": {
			expected: 100,
			got:      0,
			error:    nil,
		},
		"not equal (first equaler returns error)": {
			expected: 0,
			got:      100,
			error:    errors.New("custom equaler 1"),
		},
		"not equal (second equaler returns error)": {
			expected: false,
			got:      true,
			error:    errors.New("custom equaler 2"),
		},
		"not equal (through all custom equalers)": {
			expected: "100",
			got:      0,
			error:    errors.New("expected string (100) but got int (0)"),
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			gotError := Equal(test.expected).Assert(test.got)
			if gotError == nil {
				if test.error != nil {
					t.Errorf("expect %q but got nil", test.error)
				}
			} else {
				if test.error == nil {
					t.Errorf("expect nil but got %q", gotError)
				} else if gotError.Error() != test.error.Error() {
					t.Errorf("expect %q but got %q", test.error, gotError)
				}
			}
		})
	}
}

func TestConvert(t *testing.T) {
	tests := []struct {
		expected interface{}
		got      interface{}
		ok       bool
	}{
		{
			expected: 5,
			got:      uint64(5),
			ok:       true,
		},
		{
			expected: "test",
			got:      5,
			ok:       false,
		},
	}
	for i, test := range tests {
		test := test
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, err := convert(test.expected, test.got)
			if test.ok && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if !test.ok && err == nil {
				t.Fatal("expected error but no error")
			}
		})
	}
	t.Run("convertToBigInt", func(t *testing.T) {
		if _, err := convertToBigInt("bad value"); err == nil {
			t.Fatal("expected error but not eror")
		}
	})
	t.Run("convertToBigFloat", func(t *testing.T) {
		if _, err := convertToBigFloat("bad value"); err == nil {
			t.Fatal("expected error but not eror")
		}
	})
	t.Run("convertToInt64", func(t *testing.T) {
		if _, err := convertToInt64("bad value"); err == nil {
			t.Fatal("expected error but not eror")
		}
	})
	t.Run("convertToUint64", func(t *testing.T) {
		if _, err := convertToUint64("bad value"); err == nil {
			t.Fatal("expected error but not eror")
		}
	})
	t.Run("convertToFloat64", func(t *testing.T) {
		if _, err := convertToFloat64("bad value"); err == nil {
			t.Fatal("expected error but not eror")
		}
	})
}

func TestIsNil(t *testing.T) {
	s := "string"
	tests := map[string]struct {
		v      interface{}
		expect bool
	}{
		"nil": {
			v:      nil,
			expect: true,
		},
		"nil (string pointer)": {
			v:      (*string)(nil),
			expect: true,
		},
		"not nil (string pointer)": {
			v:      &s,
			expect: false,
		},
		"not nullable (reflect.Value.IsNil() panics)": {
			v:      s,
			expect: false,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			if got, expect := isNil(test.v), test.expect; got != expect {
				t.Fatalf("expect %t but got %t", expect, got)
			}
		})
	}
}
