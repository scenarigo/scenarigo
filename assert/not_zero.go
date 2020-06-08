package assert

import (
	"encoding/json"
	"reflect"

	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"
	"github.com/zoncoen/scenarigo/context"
)

// NotZero returns an assertion to ensure a value is not zero value.
func NotZero(q *query.Query) Assertion {
	return assertFunc(q, func(ctx *context.Context, v interface{}) error {
		if n, ok := v.(json.Number); ok {
			if i, err := n.Int64(); err == nil {
				if i == 0 {
					return errors.Errorf("%s: expected not zero value", q.String())
				}
			}
			if f, err := n.Float64(); err == nil {
				if f == 0.0 {
					return errors.Errorf("%s: expected not zero value", q.String())
				}
			}
		}
		if v == nil || reflect.DeepEqual(v, reflect.Zero(reflect.TypeOf(v)).Interface()) {
			return errors.Errorf("%s: expected not zero value", q.String())
		}
		return nil
	})
}
