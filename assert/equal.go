package assert

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"
	"github.com/zoncoen/scenarigo/context"
)

// Equal returns an assertion to ensure a value equals the expected value.
func Equal(q *query.Query, expected interface{}) Assertion {
	return assertFunc(q, func(ctx *context.Context, v interface{}) error {
		if n, ok := v.(json.Number); ok {
			switch expected.(type) {
			case int, int8, int16, int32, int64,
				uint, uint8, uint16, uint32, uint64:
				i, err := n.Int64()
				if err == nil {
					v = i
				}
			case float32, float64:
				f, err := n.Float64()
				if err == nil {
					v = f
				}
			}
		}

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
			addChildPathFromQuery(ctx, q)
			ctx.ReportYAML()
			return errors.Errorf(fmt.Sprintf("%s: expected %T (%+v) but got %T (%+v)", q.String(), expected, expected, v, v))
		}

		addChildPathFromQuery(ctx, q)
		ctx.ReportYAML()
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
		_ = recover()
	}()
	if i == nil {
		return true
	}
	return reflect.ValueOf(i).IsNil()
}
