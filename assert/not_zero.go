package assert

import (
	"encoding/json"
	"reflect"

	"github.com/scenarigo/scenarigo/errors"
)

// NotZero returns an assertion to ensure a value is not zero value.
func NotZero() Assertion {
	return AssertionFunc(func(v any) error {
		if n, ok := v.(json.Number); ok {
			if i, err := n.Int64(); err == nil {
				if i == 0 {
					return errors.New("expected not zero value")
				}
			}
			if f, err := n.Float64(); err == nil {
				if f == 0.0 {
					return errors.New("expected not zero value")
				}
			}
		}
		if v == nil || reflect.DeepEqual(v, reflect.Zero(reflect.TypeOf(v)).Interface()) {
			return errors.New("expected not zero value")
		}
		return nil
	})
}
