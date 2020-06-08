package assert

import (
	"reflect"

	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
)

// Contains returns an assertion to ensure a value contains the value.
func Contains(assertion Assertion) func(*query.Query) Assertion {
	return func(q *query.Query) Assertion {
		return assertFunc(q, func(ctx *context.Context, v interface{}) error {
			vv, err := arrayOrSlice(v)
			if err != nil {
				return errors.Wrap(err, q.String())
			}
			if err := contains(ctx, assertion, q, vv); err != nil {
				return errors.Wrapf(err, "%s: doesn't contain expected value", q.String())
			}
			return nil
		})
	}
}

// NotContains returns an assertion to ensure a value doesn't contain the value.
func NotContains(assertion Assertion) func(*query.Query) Assertion {
	return func(q *query.Query) Assertion {
		return assertFunc(q, func(ctx *context.Context, v interface{}) error {
			vv, err := arrayOrSlice(v)
			if err != nil {
				return errors.Wrap(err, q.String())
			}
			if err := contains(ctx, assertion, q, vv); err == nil {
				return errors.Errorf("%s: contains the value", q.String())
			}
			return nil
		})
	}
}

func arrayOrSlice(v interface{}) (reflect.Value, error) {
	vv := reflectutil.Elem(reflect.ValueOf(v))
	switch vv.Kind() {
	case reflect.Array, reflect.Slice:
	default:
		return reflect.Value{}, errors.New("expected an array")
	}
	return vv, nil
}

func contains(ctx *context.Context, assertion Assertion, q *query.Query, v reflect.Value) error {
	if v.Len() == 0 {
		return errors.New("empty")
	}
	var err error
	for i := 0; i < v.Len(); i++ {
		e := v.Index(i).Interface()
		if err = assertion.Assert(ctx, e); err == nil {
			return nil
		}
	}
	return errors.Wrap(err, "last error")
}
