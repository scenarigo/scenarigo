package assert

import (
	"encoding/json"
	"math/big"
	"reflect"

	"github.com/scenarigo/scenarigo/errors"
)

type compareType int

const (
	compareGreater compareType = iota
	compareGreaterOrEqual
	compareLess
	compareLessOrEqual
)

// compareNumber compares expected with actual based on compareType.
// If the comparison fails, an error will be returned.
func compareNumber(expected, actual interface{}, typ compareType) error {
	if !reflect.ValueOf(expected).IsValid() {
		return errors.Errorf("expected value %v is invalid", expected)
	}
	if !reflect.ValueOf(actual).IsValid() {
		return errors.Errorf("actual value %v is invalid", actual)
	}

	n1, err := toNumber(expected)
	if err != nil {
		return err
	}
	n2, err := toNumber(actual)
	if err != nil {
		return err
	}
	if isKindOfInt(n1) && isKindOfInt(n2) {
		i1, err := convertToBigInt(n1)
		if err != nil {
			return err
		}
		i2, err := convertToBigInt(n2)
		if err != nil {
			return err
		}
		return compareByType(i1.Cmp(i2), i2.String(), typ)
	}
	f1, err := convertToBigFloat(n1)
	if err != nil {
		return err
	}
	f2, err := convertToBigFloat(n2)
	if err != nil {
		return err
	}
	return compareByType(f1.Cmp(f2), f2.String(), typ)
}

func toNumber(v interface{}) (interface{}, error) {
	if n, ok := v.(json.Number); ok {
		if i, err := n.Int64(); err == nil {
			return i, nil
		}
		if f, err := n.Float64(); err == nil {
			return f, nil
		}
		return nil, errors.Errorf("failed to convert %v to number", n)
	}
	if !isKindOfNumber(v) {
		return nil, errors.Errorf("failed to convert %T to number", v)
	}
	return v, nil
}

func isKindOfInt(v interface{}) bool {
	switch reflect.TypeOf(v).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr:
		return true
	default:
		return false
	}
}

func isKindOfFloat(v interface{}) bool {
	switch reflect.TypeOf(v).Kind() {
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func isKindOfNumber(v interface{}) bool {
	return isKindOfInt(v) || isKindOfFloat(v)
}

func compareByType(result int, expValue string, typ compareType) error {
	switch typ {
	case compareGreater:
		if result > 0 {
			return nil
		}
		return errors.Errorf("must be greater than %s", expValue)
	case compareGreaterOrEqual:
		if result >= 0 {
			return nil
		}
		return errors.Errorf("must be equal or greater than %s", expValue)
	case compareLess:
		if result < 0 {
			return nil
		}
		return errors.Errorf("must be less than %s", expValue)
	case compareLessOrEqual:
		if result <= 0 {
			return nil
		}
		return errors.Errorf("must be equal or less than %s", expValue)
	default:
		return errors.Errorf("unknown compare type %v", typ)
	}
}

func convert[T any](v interface{}, t T) (T, error) {
	vv, err := convertToType(v, reflect.TypeOf(t))
	if err != nil {
		var zero T
		return zero, err
	}
	vvv, ok := vv.(T)
	if !ok {
		var zero T
		return zero, errors.Errorf("%T is not convertible to %T", v, t)
	}
	return vvv, nil
}

func convertToType(v interface{}, t reflect.Type) (interface{}, error) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil, errors.Errorf("value is invalid")
	}
	if rv.Type().ConvertibleTo(t) {
		return rv.Convert(t).Interface(), nil
	}
	return nil, errors.Errorf("%T is not convertible to %s", v, t)
}

func convertToInt64(v interface{}) (int64, error) {
	vv, err := convert(v, int64(0))
	if err != nil {
		return 0, err
	}
	return vv, nil
}

func convertToUint64(v interface{}) (uint64, error) {
	vv, err := convert(v, uint64(0))
	if err != nil {
		return 0, err
	}
	return vv, nil
}

func convertToFloat64(v interface{}) (float64, error) {
	vv, err := convert(v, float64(0))
	if err != nil {
		return 0, err
	}
	return vv, nil
}

func convertToBigInt(v interface{}) (*big.Int, error) {
	switch reflect.TypeOf(v).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i64, err := convertToInt64(v)
		if err != nil {
			return nil, err
		}
		return big.NewInt(i64), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u64, err := convertToUint64(v)
		if err != nil {
			return nil, err
		}
		return big.NewInt(0).SetUint64(u64), nil
	default:
		return nil, errors.Errorf("%T is not convertible to *big.Int", v)
	}
}

func convertToBigFloat(v interface{}) (*big.Float, error) {
	switch reflect.TypeOf(v).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i64, err := convertToInt64(v)
		if err != nil {
			return nil, err
		}
		return big.NewFloat(0).SetInt64(i64), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u64, err := convertToUint64(v)
		if err != nil {
			return nil, err
		}
		return big.NewFloat(0).SetUint64(u64), nil
	case reflect.Float32, reflect.Float64:
		f64, err := convertToFloat64(v)
		if err != nil {
			return nil, err
		}
		return big.NewFloat(f64), nil
	default:
		return nil, errors.Errorf("%T is not convertible to *big.Float", v)
	}
}
