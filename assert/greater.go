package assert

import (
	"reflect"

	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"
)

// Greater returns an assertion to ensure a value greater than the expected value.
func Greater(q *query.Query, expected interface{}) Assertion {
	return assertFunc(q, func(v interface{}) error {
		expectedType := reflect.TypeOf(expected)
		comparableV, err := convertComparableValue(v, expectedType)
		if err != nil {
			if q.String() == "" {
				return err
			}
			return errors.Errorf("%s: %s", q.String(), err)
		}
		ok, err := compare(expected, comparableV, compareGreater)
		if ok {
			return nil
		}
		if q.String() == "" {
			return err
		}
		return errors.Errorf("%s: %s", q.String(), err)
	})
}

// GreaterOrEqual returns an assertion to ensure a value equal or greater than the expected value.
func GreaterOrEqual(q *query.Query, expected interface{}) Assertion {
	return assertFunc(q, func(v interface{}) error {
		expectedType := reflect.TypeOf(expected)
		comparableV, err := convertComparableValue(v, expectedType)
		if err != nil {
			if q.String() == "" {
				return err
			}
			return errors.Errorf("%s: %s", q.String(), err)
		}
		ok, err := compare(expected, comparableV, compareGreaterOrEqual)
		if ok {
			return nil
		}
		if q.String() == "" {
			return err
		}
		return errors.Errorf("%s: %s", q.String(), err)
	})
}
