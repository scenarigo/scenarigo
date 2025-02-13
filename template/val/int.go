package val

import (
	"fmt"
	"math"
	"reflect"
	"strconv"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

var intType = basicType{
	name: "int",
	newValue: func(v any) (Value, error) {
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if cv, ok, _ := reflectutil.Convert(typeInt64, rv); ok {
				if f, ok := cv.Interface().(int64); ok {
					return Int(f), nil
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
				return Int(i), nil
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			i, ok := vv.Convert(typeUint64).Interface().(uint64)
			if ok {
				if i > uint64(math.MaxInt64) {
					return nil, fmt.Errorf("%d overflows int", i)
				}
				return Int(i), nil
			}
		case reflect.Float32, reflect.Float64:
			f, ok := vv.Convert(typeFloat64).Interface().(float64)
			if ok {
				if f < float64(math.MinInt64) {
					return nil, fmt.Errorf("%s overflows int", strconv.FormatFloat(f, 'f', -1, 64))
				}
				if f > float64(math.MaxInt64) {
					return nil, fmt.Errorf("%s overflows int", strconv.FormatFloat(f, 'f', -1, 64))
				}
				return Int(f), nil
			}
		case reflect.String:
			s, ok := vv.Convert(typeString).Interface().(string)
			if ok {
				i, err := strconv.ParseInt(s, 0, 64)
				if err != nil {
					return nil, err
				}
				return Int(i), nil
			}
		}
		return nil, ErrUnsupportedType
	},
}

// Int represents an int value.
type Int int64

// Type implements Value interface.
func (i Int) Type() Type {
	return intType
}

// GoValue implements Value interface.
func (i Int) GoValue() any {
	return int64(i)
}

// Neg implements Negator interface.
func (i Int) Neg() (Value, error) {
	n := int64(i)
	if n == math.MinInt64 {
		return nil, fmt.Errorf("-(%d) overflows int", n)
	}
	return Int(-n), nil
}

// Equal implements Equaler interface.
func (i Int) Equal(v Value) (LogicalValue, error) {
	if vv, ok := v.(Int); ok {
		return Bool(int64(i) == int64(vv)), nil
	}
	return Bool(false), ErrOperationNotDefined
}

// Compare implements Comparer interface.
func (i Int) Compare(v Value) (Value, error) {
	if vv, ok := v.(Int); ok {
		x := int64(i)
		y := int64(vv)
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
func (i Int) Add(v Value) (Value, error) {
	if vv, ok := v.(Int); ok {
		x := int64(i)
		y := int64(vv)
		if (y > 0 && x > math.MaxInt64-y) || (y < 0 && x < math.MinInt64-y) {
			return nil, fmt.Errorf("%d + %d overflows int", x, y)
		}
		return Int(x + y), nil
	}
	return nil, ErrOperationNotDefined
}

// Sub implements Subtractor interface.
func (i Int) Sub(v Value) (Value, error) {
	if vv, ok := v.(Int); ok {
		x := int64(i)
		y := int64(vv)
		if (y < 0 && x > math.MaxInt64+y) || (y > 0 && x < math.MinInt64+y) {
			return nil, fmt.Errorf("%d - %d overflows int", x, y)
		}
		return Int(x - y), nil
	}
	return nil, ErrOperationNotDefined
}

// Mul implements Multiplier interface.
func (i Int) Mul(v Value) (Value, error) {
	if vv, ok := v.(Int); ok {
		x := int64(i)
		y := int64(vv)
		if (x > 0 && y > 0 && x > math.MaxInt64/y) ||
			(x < 0 && y < 0 && x < math.MaxInt64/y) ||
			(x > 0 && y < 0 && y < math.MinInt64/x) ||
			(x < 0 && y > 0 && x < math.MinInt64/y) {
			return nil, fmt.Errorf("%d * %d overflows int", x, y)
		}
		return Int(x * y), nil
	}
	return nil, ErrOperationNotDefined
}

// Div implements Divider interface.
func (i Int) Div(v Value) (Value, error) {
	if vv, ok := v.(Int); ok {
		x := int64(i)
		y := int64(vv)
		if y == 0 {
			return nil, fmt.Errorf("division by 0")
		}
		return Int(x / y), nil
	}
	return nil, ErrOperationNotDefined
}

// Mod implements Modder interface.
func (i Int) Mod(v Value) (Value, error) {
	if vv, ok := v.(Int); ok {
		x := int64(i)
		y := int64(vv)
		if y == 0 {
			return nil, fmt.Errorf("division by 0")
		}
		return Int(x % y), nil
	}
	return nil, ErrOperationNotDefined
}
