package reflectutil

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
)

// ConvertStringsMap converts map[string]string to map[string][]string.
func ConvertStringsMap(v reflect.Value) (map[string][]string, error) {
	v = Elem(v)
	if !v.IsValid() {
		return nil, errors.New("invalid")
	}
	if v.Kind() != reflect.Map {
		return nil, errors.Errorf("expected map but got %T", v.Interface())
	}

	m := map[string][]string{}
	iter := v.MapRange()
	for iter.Next() {
		k := iter.Key()
		key, err := convertString(k)
		if err != nil {
			return nil, errors.Errorf("expected key is string but got %T", k.Interface())
		}

		strs, err := convertStrings(iter.Value())
		if err != nil {
			return nil, errors.Wrapf(err, "%s is invalid", key)
		}
		m[key] = strs
	}

	return m, nil
}

func convertStrings(v reflect.Value) ([]string, error) {
	if !v.IsValid() {
		return nil, errors.New("invalid")
	}
	if k := v.Kind(); k == reflect.Interface || k == reflect.Ptr {
		if v.IsNil() {
			return nil, errors.New("value is nil")
		}
	}
	v = Elem(v)
	switch v.Kind() {
	case reflect.String:
		return []string{v.String()}, nil
	case reflect.Slice:
		var strs []string
		for i := 0; i < v.Len(); i++ {
			x := v.Index(i)
			str, err := convertString(x)
			if err != nil {
				return nil, err
			}
			strs = append(strs, str)
		}
		return strs, nil
	default:
		str, err := convertString(v)
		if err == nil {
			return []string{str}, nil
		}
	}
	return nil, errors.Errorf("expected string or []string but got %T", v.Interface())
}

func convertString(v reflect.Value) (string, error) {
	if k := v.Kind(); k == reflect.Interface || k == reflect.Ptr {
		if v.IsNil() {
			return "", errors.New("value is nil")
		}
	}
	v = Elem(v)
	switch v.Kind() {
	case reflect.String:
		return v.String(), nil
	case reflect.Bool:
		return fmt.Sprintf("%t", v.Interface()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Interface()), nil
	}
	return "", errors.Errorf("expected string but got %T", v.Interface())
}
