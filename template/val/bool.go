package val

import (
	"reflect"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

var boolType = basicType{
	name: "bool",
	newValue: func(v any) (Value, error) {
		if rv := reflect.ValueOf(v); rv.Kind() == reflect.Bool {
			if cv, ok, _ := reflectutil.Convert(typeBool, rv); ok {
				if vv, ok := cv.Interface().(bool); ok {
					return Bool(vv), nil
				}
			}
		}
		return nil, ErrUnsupportedType
	},
	convert: func(v Value) (Value, error) {
		if v == nil {
			return nil, ErrUnsupportedType
		}
		if rv := reflectutil.Elem(reflect.ValueOf(v.GoValue())); rv.Kind() == reflect.Bool {
			if b, ok := rv.Convert(typeBool).Interface().(bool); ok {
				return Bool(b), nil
			}
		}
		return nil, ErrUnsupportedType
	},
}

// Bool represents a bool value.
type Bool bool

// Type implements Value interface.
func (b Bool) Type() Type {
	return boolType
}

// GoValue implements Value interface.
func (b Bool) GoValue() any {
	return bool(b)
}

// Equal implements Equaler interface.
func (b Bool) Equal(v Value) (LogicalValue, error) {
	if vv, ok := v.(Bool); ok {
		return Bool(b == vv), nil
	}
	return Bool(false), ErrOperationNotDefined
}

// IsTruthy implements LogicalValue interface.
func (b Bool) IsTruthy() bool {
	return bool(b)
}
