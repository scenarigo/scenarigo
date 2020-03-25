package template

import (
	"reflect"

	"github.com/zoncoen/scenarigo/internal/reflectutil"
	"github.com/zoncoen/yaml"
)

var yamlMapItemType = reflect.TypeOf(yaml.MapItem{})

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

func execute(v reflect.Value, data interface{}) (reflect.Value, error) {
	v = reflectutil.Elem(v)
	switch v.Kind() {
	case reflect.Map:
		for _, k := range v.MapKeys() {
			e := v.MapIndex(k)
			if !isNil(e) {
				x, err := execute(e, data)
				if err != nil {
					return reflect.Value{}, err
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
					return reflect.Value{}, err
				}
				value.Set(x)
			}
		default:
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				x, err := execute(field, data)
				if err != nil {
					return reflect.Value{}, err
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
