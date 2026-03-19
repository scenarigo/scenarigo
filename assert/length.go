package assert

import (
	"fmt"
	"reflect"

	"github.com/scenarigo/scenarigo/errors"
)

// Length returns an assertion to ensure a value length is the expected value.
func Length(expected any) Assertion {
	var assertion Assertion
	if a, ok := expected.(Assertion); ok {
		assertion = a
	} else {
		if !isKindOfInt(expected) {
			return AssertionFunc(func(v any) error {
				return fmt.Errorf("invalid expected length %#v", expected)
			})
		}
		assertion = Equal(expected)
	}
	return AssertionFunc(func(v any) error {
		if s, ok := v.(string); ok {
			v = []rune(s)
		}
		vv := reflect.ValueOf(v)
		switch vv.Kind() {
		case reflect.Array, reflect.Slice, reflect.Map:
			if err := assertion.Assert(vv.Len()); err != nil {
				return errors.Wrap(err, "length")
			}
			return nil
		default:
			return fmt.Errorf("can't get the length of %T", v)
		}
	})
}
