package val

import (
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

var durationType = basicType{
	name: "duration",
	newValue: func(v any) (Value, error) {
		if d, ok := v.(time.Duration); ok {
			return Duration(d), nil
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
			if i, ok := vv.Convert(typeInt64).Interface().(int64); ok {
				return Duration(i), nil
			}
		case reflect.String:
			if s, ok := vv.Convert(typeString).Interface().(string); ok {
				d, err := time.ParseDuration(s)
				if err != nil {
					return nil, fmt.Errorf("can't convert string to duration: %w", err)
				}
				return Duration(d), nil
			}
		}
		return nil, ErrUnsupportedType
	},
}

// Duration represents a duration value.
type Duration time.Duration

// Type implements Value interface.
func (d Duration) Type() Type {
	return durationType
}

// GoValue implements Value interface.
func (d Duration) GoValue() any {
	return time.Duration(d)
}

// Neg implements Negator interface.
func (d Duration) Neg() (Value, error) {
	n := int64(d)
	if n == math.MinInt64 {
		return nil, fmt.Errorf("-(%d) overflows duration", n)
	}
	return Duration(-n), nil
}

// Equal implements Equaler interface.
func (d Duration) Equal(v Value) (LogicalValue, error) {
	if e, ok := v.GoValue().(time.Duration); ok {
		return Bool(time.Duration(d) == e), nil
	}
	return Bool(false), ErrOperationNotDefined
}

// Equal implements Equaler interface.
func (d Duration) Compare(v Value) (Value, error) {
	if y, ok := v.GoValue().(time.Duration); ok {
		x := time.Duration(d)
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
func (d Duration) Add(v Value) (Value, error) {
	x := time.Duration(d)
	switch y := v.GoValue().(type) {
	case time.Duration:
		return Duration(x + y), nil
	case time.Time:
		return Time(y.Add(x)), nil
	}
	return nil, ErrOperationNotDefined
}

// Sub implements Subtractor interface.
func (d Duration) Sub(v Value) (Value, error) {
	if y, ok := v.GoValue().(time.Duration); ok {
		x := time.Duration(d)
		return Duration(x - y), nil
	}
	return nil, ErrOperationNotDefined
}
