package val

import (
	"fmt"
	"reflect"
	"time"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

var timeType = basicType{
	name: "time",
	newValue: func(v any) (Value, error) {
		if t, ok := v.(time.Time); ok {
			return Time(t), nil
		}
		return nil, ErrUnsupportedType
	},
	convert: func(v Value) (Value, error) {
		if v == nil {
			return nil, ErrUnsupportedType
		}
		rv := reflectutil.Elem(reflect.ValueOf(v.GoValue()))
		switch rv.Kind() {
		case reflect.String:
			s, ok := rv.Convert(typeString).Interface().(string)
			if ok {
				t, err := time.Parse(time.RFC3339, s)
				if err != nil {
					return nil, fmt.Errorf("can't convert string to time: %w", err)
				}
				return Time(t), nil
			}
		case reflect.Struct:
			if v, ok, _ := reflectutil.Convert(typeTime, rv); ok {
				if t, ok := v.Interface().(time.Time); ok {
					return Time(t), nil
				}
			}
		}
		return nil, ErrUnsupportedType
	},
}

// Time represents a time value.
type Time time.Time

// Type implements Value interface.
func (t Time) Type() Type {
	return timeType
}

// GoValue implements Value interface.
func (t Time) GoValue() any {
	return time.Time(t)
}

// Equal implements Equaler interface.
func (t Time) Equal(v Value) (LogicalValue, error) {
	if u, ok := v.GoValue().(time.Time); ok {
		return Bool(time.Time(t).Equal(u)), nil
	}
	return Bool(false), ErrOperationNotDefined
}

// Compare implements Comparer interface.
func (t Time) Compare(v Value) (Value, error) {
	if y, ok := v.GoValue().(time.Time); ok {
		x := time.Time(t)
		if x.Before(y) {
			return Int(-1), nil
		}
		if x.After(y) {
			return Int(1), nil
		}
		return Int(0), nil
	}
	return nil, ErrOperationNotDefined
}

// Add implements Adder interface.
func (t Time) Add(v Value) (Value, error) {
	if y, ok := v.GoValue().(time.Duration); ok {
		x := time.Time(t)
		return Time(x.Add(y)), nil
	}
	return nil, ErrOperationNotDefined
}

// Sub implements Subtractor interface.
func (t Time) Sub(v Value) (Value, error) {
	x := time.Time(t)
	switch y := v.GoValue().(type) {
	case time.Time:
		return Duration(x.Sub(y)), nil
	case time.Duration:
		return Time(x.Add(-y)), nil
	}
	return nil, ErrOperationNotDefined
}
