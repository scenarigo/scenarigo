package val

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

var floatType = basicType{
	name: "float",
	newValue: func(v any) (Value, error) {
		rv := reflectutil.Elem(reflect.ValueOf(v))
		switch rv.Kind() {
		case reflect.Float32, reflect.Float64:
			if cv, ok, _ := reflectutil.Convert(typeFloat64, rv); ok {
				if f, ok := cv.Interface().(float64); ok {
					return Float(f), nil
				}
			}
		}
		return nil, ErrUnsupportedType
	},
	convert: func(v Value) (Value, error) {
		if v == nil {
			return nil, ErrUnsupportedType
		}
		vv := reflectutil.Elem(reflect.ValueOf(v.GoValue()))
		switch vv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			i, ok := vv.Convert(typeInt64).Interface().(int64)
			if ok {
				return Float(i), nil
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			i, ok := vv.Convert(typeUint64).Interface().(uint64)
			if ok {
				return Float(i), nil
			}
		case reflect.Float32, reflect.Float64:
			f, ok := vv.Convert(typeFloat64).Interface().(float64)
			if ok {
				return Float(f), nil
			}
		case reflect.String:
			s, ok := vv.Convert(typeString).Interface().(string)
			if ok {
				f, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return nil, err
				}
				return Float(f), nil
			}
		}
		return nil, ErrUnsupportedType
	},
}

// Float represents a float value.
type Float float64

// Type implements Value interface.
func (f Float) Type() Type {
	return floatType
}

// GoValue implements Value interface.
func (f Float) GoValue() any {
	return float64(f)
}

// Neg implements Negator interface.
func (f Float) Neg() (Value, error) {
	return Float(-float64(f)), nil
}

// Equal implements Equaler interface.
func (f Float) Equal(v Value) (LogicalValue, error) {
	if vv, ok := v.(Float); ok {
		return Bool(float64(f) == float64(vv)), nil
	}
	return Bool(false), ErrOperationNotDefined
}

// Compare implements Comparer interface.
func (f Float) Compare(v Value) (Value, error) {
	if vv, ok := v.(Float); ok {
		x := float64(f)
		y := float64(vv)
		if x < y {
			return Int(-1), nil
		}
		if x > y {
			return Int(1), nil
		}
		return Int(0), nil
	}
	return nil, ErrOperationNotDefined
}

// Add implements Adder interface.
func (f Float) Add(v Value) (Value, error) {
	if vv, ok := v.(Float); ok {
		x := float64(f)
		y := float64(vv)
		return Float(x + y), nil
	}
	return nil, ErrOperationNotDefined
}

// Sub implements Subtractor interface.
func (f Float) Sub(v Value) (Value, error) {
	if vv, ok := v.(Float); ok {
		x := float64(f)
		y := float64(vv)
		return Float(x - y), nil
	}
	return nil, ErrOperationNotDefined
}

// Mul implements Multiplier interface.
func (f Float) Mul(v Value) (Value, error) {
	if vv, ok := v.(Float); ok {
		x := float64(f)
		y := float64(vv)
		return Float(x * y), nil
	}
	return nil, ErrOperationNotDefined
}

// Div implements Divider interface.
func (f Float) Div(v Value) (Value, error) {
	if vv, ok := v.(Float); ok {
		x := float64(f)
		y := float64(vv)
		if y == 0.0 {
			return nil, fmt.Errorf("division by 0")
		}
		return Float(x / y), nil
	}
	return nil, ErrOperationNotDefined
}
