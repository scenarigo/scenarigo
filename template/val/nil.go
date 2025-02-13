package val

import (
	"reflect"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

var nilType = basicType{
	name: "nil",
	newValue: func(v any) (Value, error) {
		if rv := reflectutil.Elem(reflect.ValueOf(v)); rv.Kind() == reflect.Invalid {
			return Nil{v}, nil
		}
		return nil, ErrUnsupportedType
	},
	convert: func(v Value) (Value, error) {
		if v == nil {
			return Nil{nil}, nil
		}
		if rv := reflectutil.Elem(reflect.ValueOf(v.GoValue())); rv.Kind() == reflect.Invalid {
			return Nil{v.GoValue()}, nil
		}
		return nil, ErrUnsupportedType
	},
}

// Nil represents a nil value.
type Nil struct {
	v any
}

// Type implements Value interface.
func (n Nil) Type() Type {
	return nilType
}

// GoValue implements Value interface.
func (n Nil) GoValue() any { return n.v }

// Equal implements Equaler interface.
func (n Nil) Equal(v Value) (LogicalValue, error) {
	if _, ok := v.(Nil); ok {
		return Bool(true), nil
	}
	return Bool(false), ErrOperationNotDefined
}
