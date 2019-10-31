package assert

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"
)

// Equal returns an assertion to ensure a value equals the expected value.
func Equal(q *query.Query, expected interface{}) Assertion {
	return assertFunc(q, func(v interface{}) error {
		if reflect.DeepEqual(v, expected) {
			return nil
		}

		if isNil(v) && isNil(expected) {
			return nil
		}

		if t := reflect.TypeOf(v); t != reflect.TypeOf(expected) {
			// handle enumeration strings
			if s, ok := expected.(string); ok {
				if enum, ok := v.(interface {
					String() string
					EnumDescriptor() ([]byte, []int)
				}); ok {
					if enum.String() == s {
						return nil
					}
				}
			}
			// try type conversion
			converted, err := convert(expected, t)
			if err == nil {
				if reflect.DeepEqual(v, converted) {
					return nil
				}
			}
			return errors.Errorf(fmt.Sprintf("%s: expected %T (%+v) but got %T (%+v)", q.String(), expected, expected, v, v))
		}

		return errors.Errorf(fmt.Sprintf("%s: expected %+v but got %+v", q.String(), expected, v))
	})
}

func convert(v interface{}, t reflect.Type) (result interface{}, resErr error) {
	defer func() {
		if err := recover(); err != nil {
			resErr = errors.Errorf("failed to convert: %s", err)
		}
	}()
	result = reflect.ValueOf(v).Convert(t).Interface()
	return
}

func isNil(i interface{}) bool {
	defer func() {
		// return false if IsNil panics
		recover()
	}()
	if i == nil {
		return true
	}
	return reflect.ValueOf(i).IsNil()
}
