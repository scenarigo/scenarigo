package assert

import (
	"encoding/json"
	"reflect"

	"github.com/zoncoen/scenarigo/errors"
)

func convertComparableValue(v interface{}, t reflect.Type) (interface{}, error) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil, errors.Errorf("v is invalid value")
	}
	if t == nil {
		return nil, errors.Errorf("expected type is nil")
	}

	// if v is json.Number, we try to convert expected type.
	if n, ok := v.(json.Number); ok {
		return convertJSONNumber(n, t)
	}

	if t.Kind() == reflect.String {
		enum, ok := v.(interface {
			String() string
			EnumDescriptor() ([]byte, []int)
		})
		if ok {
			return enum.String(), nil
		}
	}

	if rv.Type().ConvertibleTo(t) {
		return rv.Convert(t).Interface(), nil
	}

	return nil, errors.Errorf("failed to convert %T to %s", v, t)
}

func convertJSONNumberToInt(v json.Number) (int64, error) {
	i, err := v.Int64()
	if err == nil {
		return i, nil
	}
	// if json.Number is float, Int64 will return an error because it uses strconv.ParseInt internally.
	f, err := v.Float64()
	if err == nil {
		return int64(f), nil
	}
	return 0, errors.Errorf("cannot convert json.Number to integer type")
}

func convertJSONNumber(v json.Number, t reflect.Type) (interface{}, error) {
	switch t.Kind() {
	case reflect.Int:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return int(i), nil
	case reflect.Int8:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return int8(i), nil
	case reflect.Int16:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return int16(i), nil
	case reflect.Int32:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return int32(i), nil
	case reflect.Int64:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return i, nil
	case reflect.Uint:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return uint(i), nil
	case reflect.Uint8:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return uint8(i), nil
	case reflect.Uint16:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return uint16(i), nil
	case reflect.Uint32:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return uint32(i), nil
	case reflect.Uint64:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return uint64(i), nil
	case reflect.Uintptr:
		i, err := convertJSONNumberToInt(v)
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return uintptr(i), nil
	case reflect.Float32:
		f, err := v.Float64()
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return float32(f), nil
	case reflect.Float64:
		f, err := v.Float64()
		if err != nil {
			return nil, errors.Errorf("expected %s type. but %v couldn't convert it: %s", t, v, err)
		}
		return f, nil
	case reflect.String:
		return v.String(), nil
	}
	return nil, errors.Errorf("failed to convert %T to %s", v, t)
}

type compareType int

func (t compareType) String() string {
	switch t {
	case compareEqual:
		return "equal"
	case compareGreater:
		return "greater"
	case compareGreaterOrEqual:
		return "greater or equal"
	case compareLess:
		return "less"
	case compareLessOrEqual:
		return "less or equal"
	}
	return ""
}

const (
	compareEqual compareType = iota
	compareGreater
	compareGreaterOrEqual
	compareLess
	compareLessOrEqual
)

func compare(v, v2 interface{}, typ compareType) (bool, error) {
	if reflect.TypeOf(v) != reflect.TypeOf(v2) {
		return false, errors.Errorf("expected type is %T but got type is %T", v, v2)
	}
	switch vv := v.(type) {
	case int:
		vv2 := v2.(int)
		return compareInt(int64(vv), int64(vv2), typ)
	case int8:
		vv2 := v2.(int8)
		return compareInt(int64(vv), int64(vv2), typ)
	case int16:
		vv2 := v2.(int16)
		return compareInt(int64(vv), int64(vv2), typ)
	case int32:
		vv2 := v2.(int32)
		return compareInt(int64(vv), int64(vv2), typ)
	case int64:
		vv2 := v2.(int64)
		return compareInt(vv, vv2, typ)
	case uint:
		vv2 := v2.(uint)
		return compareUint(uint64(vv), uint64(vv2), typ)
	case uint8:
		vv2 := v2.(uint8)
		return compareUint(uint64(vv), uint64(vv2), typ)
	case uint16:
		vv2 := v2.(uint16)
		return compareUint(uint64(vv), uint64(vv2), typ)
	case uint32:
		vv2 := v2.(uint32)
		return compareUint(uint64(vv), uint64(vv2), typ)
	case uint64:
		vv2 := v2.(uint64)
		return compareUint(vv, vv2, typ)
	case uintptr:
		vv2 := v2.(uintptr)
		return compareUint(uint64(vv), uint64(vv2), typ)
	case float32:
		vv2 := v2.(float32)
		return compareFloat(float64(vv), float64(vv2), typ)
	case float64:
		vv2 := v2.(float64)
		return compareFloat(float64(vv), float64(vv2), typ)
	case string:
		vv2 := v2.(string)
		return compareString(vv, vv2, typ)
	}
	return false, errors.Errorf("expected type is %T but it doesn't compare by %s", v, typ)
}

func compareInt(v, v2 int64, typ compareType) (bool, error) {
	switch typ {
	case compareEqual:
		if v == v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal %v", v, v2)
	case compareGreater:
		if v > v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not greater than %v", v, v2)
	case compareGreaterOrEqual:
		if v >= v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal or greater than %v", v, v2)
	case compareLess:
		if v < v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not less than %v", v, v2)
	case compareLessOrEqual:
		if v <= v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal of less than %v", v, v2)
	}
	return false, errors.Errorf("unknown compare type %s", typ)
}

func compareUint(v, v2 uint64, typ compareType) (bool, error) {
	switch typ {
	case compareEqual:
		if v == v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal %v", v, v2)
	case compareGreater:
		if v > v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not greater than %v", v, v2)
	case compareGreaterOrEqual:
		if v >= v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal or greater than %v", v, v2)
	case compareLess:
		if v < v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not less than %v", v, v2)
	case compareLessOrEqual:
		if v <= v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal of less than %v", v, v2)
	}
	return false, errors.Errorf("unknown compare type %s", typ)
}

func compareFloat(v, v2 float64, typ compareType) (bool, error) {
	switch typ {
	case compareEqual:
		if v == v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal %v", v, v2)
	case compareGreater:
		if v > v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not greater than %v", v, v2)
	case compareGreaterOrEqual:
		if v >= v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal or greater than %v", v, v2)
	case compareLess:
		if v < v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not less than %v", v, v2)
	case compareLessOrEqual:
		if v <= v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal of less than %v", v, v2)
	}
	return false, errors.Errorf("unknown compare type %s", typ)
}

func compareString(v, v2 string, typ compareType) (bool, error) {
	switch typ {
	case compareEqual:
		if v == v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal %v", v, v2)
	case compareGreater:
		if v > v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not greater than %v", v, v2)
	case compareGreaterOrEqual:
		if v >= v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal or greater than %v", v, v2)
	case compareLess:
		if v < v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not less than %v", v, v2)
	case compareLessOrEqual:
		if v <= v2 {
			return true, nil
		}
		return false, errors.Errorf("%v is not equal of less than %v", v, v2)
	}
	return false, errors.Errorf("unknown compare type %s", typ)
}
