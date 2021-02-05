package assert

import (
	"reflect"
)

// Greater returns an assertion to ensure a value greater than the expected value.
func Greater(expected interface{}) Assertion {
	return AssertionFunc(func(v interface{}) error {
		actual, err := convertComparableValue(v, reflect.TypeOf(expected))
		if err != nil {
			return err
		}
		ok, err := compare(actual, expected, compareGreater)
		if ok {
			return nil
		}
		return err
	})
}

// GreaterOrEqual returns an assertion to ensure a value equal or greater than the expected value.
func GreaterOrEqual(expected interface{}) Assertion {
	return AssertionFunc(func(v interface{}) error {
		actual, err := convertComparableValue(v, reflect.TypeOf(expected))
		if err != nil {
			return err
		}
		ok, err := compare(actual, expected, compareGreaterOrEqual)
		if ok {
			return nil
		}
		return err
	})
}
