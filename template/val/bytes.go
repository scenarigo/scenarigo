package val

import (
	"bytes"
	"reflect"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

var bytesType = basicType{
	name: "bytes",
	newValue: func(v any) (Value, error) {
		if b, ok := v.([]byte); ok {
			return Bytes(b), nil
		}
		return nil, ErrUnsupportedType
	},
	convert: func(v Value) (Value, error) {
		if v == nil {
			return nil, ErrUnsupportedType
		}
		rv := reflectutil.Elem(reflect.ValueOf(v.GoValue()))
		switch rv.Kind() {
		case reflect.Slice:
			if rv.Type().Elem().Kind() == reflect.Uint8 {
				if bv, ok, _ := reflectutil.Convert(typeBytes, rv); ok {
					if b, ok := bv.Interface().([]byte); ok {
						return Bytes(b), nil
					}
				}
			}
		case reflect.String:
			if s, ok := rv.Convert(typeString).Interface().(string); ok {
				return Bytes(s), nil
			}
		}
		return nil, ErrUnsupportedType
	},
}

// Bytes represents a bytes value.
type Bytes []byte

// Type implements Value interface.
func (b Bytes) Type() Type {
	return bytesType
}

// GoValue implements Value interface.
func (b Bytes) GoValue() any {
	return []byte(b)
}

// Equal implements Equaler interface.
func (b Bytes) Equal(v Value) (LogicalValue, error) {
	if vv, ok := v.(Bytes); ok {
		return Bool(bytes.Equal([]byte(b), []byte(vv))), nil
	}
	return Bool(false), ErrOperationNotDefined
}

// Compare implements Comparer interface.
func (b Bytes) Compare(v Value) (Value, error) {
	if vv, ok := v.(Bytes); ok {
		return Int(bytes.Compare([]byte(b), []byte(vv))), nil
	}
	return nil, ErrOperationNotDefined
}

// Add implements Adder interface.
func (b Bytes) Add(v Value) (Value, error) {
	if vv, ok := v.(Bytes); ok {
		return Bytes(append([]byte(b), []byte(vv)...)), nil
	}
	return nil, ErrOperationNotDefined
}

// Size implements Sizer interface.
func (b Bytes) Size() (Value, error) {
	return Int(len(b)), nil
}
