package template

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/zoncoen/scenarigo/errors"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
)

var (
	yamlMapItemType  = reflect.TypeOf(yaml.MapItem{})
	yamlMapSliceType = reflect.TypeOf(yaml.MapSlice{})
)

// Execute executes templates of i with data.
func Execute(i, data interface{}) (interface{}, error) {
	v, err := execute(reflect.ValueOf(i), data)
	if err != nil {
		return nil, err
	}
	if v.IsValid() {
		return v.Interface(), nil
	}
	return nil, nil
}

func structFieldName(field reflect.StructField) string {
	fieldName := field.Name
	tag := field.Tag.Get("yaml")
	if tag == "" {
		return fieldName
	}

	tagValues := strings.Split(tag, ",")
	if len(tagValues) > 0 && tagValues[0] != "" {
		return tagValues[0]
	}
	return fieldName
}

func execute(v reflect.Value, data interface{}) (reflect.Value, error) {
	v = reflectutil.Elem(v)
	switch v.Kind() {
	case reflect.Map:
		for _, k := range v.MapKeys() {
			e := v.MapIndex(k)
			if !isNil(e) {
				x, err := execute(e, data)
				if err != nil {
					key := fmt.Sprint(k.Interface())
					return reflect.Value{}, errors.WithPath(err, key)
				}
				v.SetMapIndex(k, x)
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			e := v.Index(i)
			if !isNil(e) {
				x, err := execute(e, data)
				if err != nil {
					if v.Type() != yamlMapSliceType {
						err = errors.WithPath(err, fmt.Sprintf("[%d]", i))
					}
					return reflect.Value{}, err
				}
				e.Set(x)
			}
		}
	case reflect.Struct:
		switch v.Type() {
		case yamlMapItemType:
			value := v.FieldByName("Value")
			if !isNil(value) {
				x, err := execute(value, data)
				if err != nil {
					key := fmt.Sprint(v.FieldByName("Key").Interface())
					return reflect.Value{}, errors.WithPath(err, key)
				}
				value.Set(x)
			}
		default:
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				x, err := execute(field, data)
				if err != nil {
					fieldName := structFieldName(v.Type().Field(i))
					return reflect.Value{}, errors.WithPath(err, fieldName)
				}
				field.Set(x)
			}
		}
	case reflect.String:
		tmpl, err := New(v.String())
		if err != nil {
			return reflect.Value{}, err
		}
		x, err := tmpl.Execute(data)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(x), nil
	}
	return v, nil
}

func isNil(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.IsNil()
	}
	return false
}
