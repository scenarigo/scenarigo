package assertutil

import (
	"fmt"
	"reflect"

	"github.com/goccy/go-yaml"

	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

// BuildHeaderAssertion builds an assertion for headers.
func BuildHeaderAssertion(ctx *context.Context, in yaml.MapSlice) (assert.Assertion, error) {
	expects := make(yaml.MapSlice, len(in))
	opts := []assert.BuildOpt{
		assert.FromTemplate(ctx),
		assert.WithEqualers(assert.EqualerFunc(func(x, y any) (bool, error) {
			// Convert boolean and integer values to strings for ease of use.
			// All header values are strings.
			x = stringify(x)

			return true, assert.Equal(x).Assert(y)
		})),
	}
	for i, elem := range in {
		k, ok := stringify(elem.Key).(string)
		if !ok {
			return nil, errors.Errorf("name must be string but %T", elem.Key)
		}
		valAssertion, err := assert.Build(ctx.RequestContext(), elem.Value, opts...)
		if err != nil {
			return nil, errors.WithPath(err, k)
		}

		// Wrap with the "Contains" function to allow using not only an array but also just a string.
		if reflect.ValueOf(elem.Value).Kind() != reflect.Slice {
			valAssertion = assert.Contains(valAssertion)
		}

		elem.Value = valAssertion
		expects[i] = elem
	}
	return assert.Build(ctx.RequestContext(), expects, opts...)
}

func stringify(i any) any {
	switch v := i.(type) {
	case yaml.MapSlice:
		for i, item := range v {
			v[i].Value = stringify(item.Value)
		}
		return v
	case []any:
		for i, elm := range v {
			v[i] = stringify(elm)
		}
		return v
	}

	v := reflectutil.Elem(reflect.ValueOf(i))
	switch v.Kind() {
	case
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64:
		return fmt.Sprint(v.Interface())
	default:
		return i
	}
}
