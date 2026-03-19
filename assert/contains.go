package assert

import (
	"reflect"

	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

// Contains returns an assertion to ensure a value contains the value.
func Contains(assertion Assertion) Assertion {
	return AssertionFunc(func(v any) error {
		vv, err := arrayOrSlice(v)
		if err != nil {
			return err
		}
		if err := contains(assertion, vv); err != nil {
			return errors.Wrap(err, "doesn't contain expected value")
		}
		return nil
	})
}

// NotContains returns an assertion to ensure a value doesn't contain the value.
func NotContains(assertion Assertion) Assertion {
	return AssertionFunc(func(v any) error {
		vv, err := arrayOrSlice(v)
		if err != nil {
			return err
		}
		if err := contains(assertion, vv); err == nil {
			return errors.ErrorQueryf(queryutil.New(), "contains the value")
		}
		return nil
	})
}

func arrayOrSlice(v any) (reflect.Value, error) {
	vv := reflectutil.Elem(reflect.ValueOf(v))
	switch vv.Kind() {
	case reflect.Array, reflect.Slice:
	default:
		return reflect.Value{}, errors.New("expected an array")
	}
	return vv, nil
}

func contains(assertion Assertion, v reflect.Value) error {
	if v.Len() == 0 {
		return errors.New("empty")
	}
	var err error
	for i := range v.Len() {
		e := v.Index(i).Interface()
		if err = assertion.Assert(e); err == nil {
			return nil
		}
	}
	return errors.Wrap(err, "last error")
}
