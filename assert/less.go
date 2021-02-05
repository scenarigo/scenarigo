package assert

import (
	"reflect"

	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"
)

// Less returns an assertion to ensure a value less than the expected value.
func Less(q *query.Query, expected interface{}) Assertion {
	return assertFunc(q, func(v interface{}) error {
		expectedType := reflect.TypeOf(expected)
		comparableV, err := convertComparableValue(v, expectedType)
		if err != nil {
			if q.String() == "" {
				return err
			}
			return errors.Errorf("%s: %s", q.String(), err)
		}
		ok, err := compare(expected, comparableV, compareLess)
		if ok {
			return nil
		}
		if q.String() == "" {
			return err
		}
		return errors.Errorf("%s: %s", q.String(), err)
	})
}

// LessOrEqual returns an assertion to ensure a value equal or less than the expected value.
func LessOrEqual(q *query.Query, expected interface{}) Assertion {
	return assertFunc(q, func(v interface{}) error {
		expectedType := reflect.TypeOf(expected)
		comparableV, err := convertComparableValue(v, expectedType)
		if err != nil {
			if q.String() == "" {
				return err
			}
			return errors.Errorf("%s: %s", q.String(), err)
		}
		ok, err := compare(expected, comparableV, compareLessOrEqual)
		if ok {
			return nil
		}
		if q.String() == "" {
			return err
		}
		return errors.Errorf("%s: %s", q.String(), err)
	})
}
