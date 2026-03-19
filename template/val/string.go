package val

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

var stringType = basicType{
	name: "string",
	newValue: func(v any) (Value, error) {
		if rv := reflect.ValueOf(v); rv.Kind() == reflect.String {
			if cv, ok, _ := reflectutil.Convert(typeString, rv); ok {
				if vv, ok := cv.Interface().(string); ok {
					return String(vv), nil
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
			if d, ok := vv.Interface().(time.Duration); ok {
				return String(d.String()), nil
			}
			i, ok := vv.Convert(typeInt64).Interface().(int64)
			if ok {
				return String(strconv.FormatInt(i, 10)), nil
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			i, ok := vv.Convert(typeUint64).Interface().(uint64)
			if ok {
				return String(strconv.FormatUint(i, 10)), nil
			}
		case reflect.Float32, reflect.Float64:
			f, ok := vv.Convert(typeFloat64).Interface().(float64)
			if ok {
				return String(strconv.FormatFloat(f, 'f', -1, 64)), nil
			}
		case reflect.String:
			s, ok := vv.Convert(typeString).Interface().(string)
			if ok {
				return String(s), nil
			}
		case reflect.Slice:
			if vv.Type().Elem().Kind() == reflect.Uint8 {
				if bv, ok, _ := reflectutil.Convert(typeBytes, vv); ok {
					if b, ok := bv.Interface().([]byte); ok {
						if !utf8.Valid(b) {
							return nil, fmt.Errorf("can't convert bytes to string: invalid UTF-8 encoded characters in bytes")
						}
						return String(string(b)), nil
					}
				}
			}
		case reflect.Struct:
			if vv, ok, _ := reflectutil.Convert(typeTime, vv); ok {
				t, ok := vv.Interface().(time.Time)
				if ok {
					return String(t.Format(time.RFC3339)), nil
				}
			}
		}
		return nil, ErrUnsupportedType
	},
}

// String represents a string value.
type String string

// Type implements Value interface.
func (s String) Type() Type {
	return stringType
}

// GoValue implements Value interface.
func (s String) GoValue() any {
	return string(s)
}

// Equal implements Equaler interface.
func (s String) Equal(v Value) (LogicalValue, error) {
	if vv, ok := v.(String); ok {
		return Bool(string(s) == string(vv)), nil
	}
	return Bool(false), ErrOperationNotDefined
}

// Compare implements Comparer interface.
func (s String) Compare(v Value) (Value, error) {
	if vv, ok := v.(String); ok {
		return Int(strings.Compare(string(s), string(vv))), nil
	}
	return nil, ErrOperationNotDefined
}

// Add implements Adder interface.
func (s String) Add(v Value) (Value, error) {
	if vv, ok := v.(String); ok {
		x := string(s)
		y := string(vv)
		return String(x + y), nil
	}
	return nil, ErrOperationNotDefined
}

// Size implements Sizer interface.
func (s String) Size() (Value, error) {
	return Int(utf8.RuneCountInString(string(s))), nil
}
