package val

import (
	"fmt"
	"math"
	"reflect"
	"strconv"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

var uintType = basicType{
	name: "uint",
	newValue: func(v any) (Value, error) {
		rv := reflectutil.Elem(reflect.ValueOf(v))
		switch rv.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if cv, ok, _ := reflectutil.Convert(typeUint64, rv); ok {
				if i, ok := cv.Interface().(uint64); ok {
					return Uint(i), nil
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
				if i < 0 {
					return nil, fmt.Errorf("can't convert %d to uint", i)
				}
				return Uint(i), nil
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			i, ok := vv.Convert(typeUint64).Interface().(uint64)
			if ok {
				return Uint(i), nil
			}
		case reflect.Float32, reflect.Float64:
			f, ok := vv.Convert(typeFloat64).Interface().(float64)
			if ok {
				if f < 0 {
					return nil, fmt.Errorf("can't convert %s to uint", strconv.FormatFloat(f, 'f', -1, 64))
				}
				if f > float64(math.MaxUint64) {
					return nil, fmt.Errorf("%s overflows uint", strconv.FormatFloat(f, 'f', -1, 64))
				}
				return Uint(f), nil
			}
		case reflect.String:
			s, ok := vv.Convert(typeString).Interface().(string)
			if ok {
				i, err := strconv.ParseUint(s, 0, 64)
				if err != nil {
					return nil, err
				}
				return Uint(i), nil
			}
		}
		return nil, ErrUnsupportedType
	},
}

// Uint represents a uint value.
type Uint uint64

// Type implements Value interface.
func (i Uint) Type() Type {
	return uintType
}

// GoValue implements Value interface.
func (i Uint) GoValue() any {
	return uint64(i)
}

// Equal implements Equaler interface.
func (i Uint) Equal(v Value) (LogicalValue, error) {
	if vv, ok := v.(Uint); ok {
		return Bool(uint64(i) == uint64(vv)), nil
	}
	return Bool(false), ErrOperationNotDefined
}

// Compare implements Comparer interface.
func (i Uint) Compare(v Value) (Value, error) {
	if vv, ok := v.(Uint); ok {
		x := uint64(i)
		y := uint64(vv)
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
func (i Uint) Add(v Value) (Value, error) {
	if vv, ok := v.(Uint); ok {
		x := uint64(i)
		y := uint64(vv)
		if y > 0 && x > math.MaxUint64-y {
			return nil, fmt.Errorf("%d + %d overflows uint", x, y)
		}
		return Uint(x + y), nil
	}
	return nil, ErrOperationNotDefined
}

// Sub implements Subtractor interface.
func (i Uint) Sub(v Value) (Value, error) {
	if vv, ok := v.(Uint); ok {
		x := uint64(i)
		y := uint64(vv)
		if x < y {
			return nil, fmt.Errorf("%d - %d overflows uint", x, y)
		}
		return Uint(x - y), nil
	}
	return nil, ErrOperationNotDefined
}

// Mul implements Multiplier interface.
func (i Uint) Mul(v Value) (Value, error) {
	if vv, ok := v.(Uint); ok {
		x := uint64(i)
		y := uint64(vv)
		if x > math.MaxUint64/y {
			return nil, fmt.Errorf("%d * %d overflows uint", x, y)
		}
		return Uint(x * y), nil
	}
	return nil, ErrOperationNotDefined
}

// Div implements Divider interface.
func (i Uint) Div(v Value) (Value, error) {
	if vv, ok := v.(Uint); ok {
		x := uint64(i)
		y := uint64(vv)
		if y == 0 {
			return nil, fmt.Errorf("division by 0")
		}
		return Uint(x / y), nil
	}
	return nil, ErrOperationNotDefined
}

// Mod implements Modder interface.
func (i Uint) Mod(v Value) (Value, error) {
	if vv, ok := v.(Uint); ok {
		x := uint64(i)
		y := uint64(vv)
		if y == 0 {
			return nil, fmt.Errorf("division by 0")
		}
		return Uint(x % y), nil
	}
	return nil, ErrOperationNotDefined
}
