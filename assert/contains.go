package assert

import (
	"reflect"

	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
)

// Contains returns an assertion to ensure a value contains the value.
func Contains(assertion Assertion) func(*query.Query) Assertion {
	return func(q *query.Query) Assertion {
		return assertFunc(q, func(v interface{}) error {
			ok, err := contains(assertion, q, v)
			if err != nil {
				return err
			}
			if !ok {
				return errors.Errorf("%s: doesn't contain expected value", q.String())
			}
			return nil
		})
	}
}

// NotContains returns an assertion to ensure a value doesn't contain the value.
func NotContains(assertion Assertion) func(*query.Query) Assertion {
	return func(q *query.Query) Assertion {
		return assertFunc(q, func(v interface{}) error {
			ok, err := contains(assertion, q, v)
			if err != nil {
				return err
			}
			if ok {
				return errors.Errorf("%s: contains the value", q.String())
			}
			return nil
		})
	}
}

func contains(assertion Assertion, q *query.Query, v interface{}) (bool, error) {
	vv := reflectutil.Elem(reflect.ValueOf(v))
	switch vv.Kind() {
	case reflect.Array, reflect.Slice:
	default:
		return false, errors.Errorf("%s: expected an array or slice", q.String())
	}
	var err error
	for i := 0; i < vv.Len(); i++ {
		e := vv.Index(i).Interface()
		if err = assertion.Assert(e); err == nil {
			return true, nil
		}
	}
	return false, nil
}
