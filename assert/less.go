package assert

import (
	"reflect"
)

// Less returns an assertion to ensure a value less than the expected value.
func Less(expected interface{}) Assertion {
	return AssertionFunc(func(v interface{}) error {
		actual, err := convertComparableValue(v, reflect.TypeOf(expected))
		if err != nil {
			return err
		}
		ok, err := compare(actual, expected, compareLess)
		if ok {
			return nil
		}
		return err
	})
}

// LessOrEqual returns an assertion to ensure a value equal or less than the expected value.
func LessOrEqual(expected interface{}) Assertion {
	return AssertionFunc(func(v interface{}) error {
		actual, err := convertComparableValue(v, reflect.TypeOf(expected))
		if err != nil {
			return err
		}
		ok, err := compare(actual, expected, compareLessOrEqual)
		if ok {
			return nil
		}
		return err
	})
}
