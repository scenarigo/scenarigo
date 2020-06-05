package template

import (
	"fmt"
	"reflect"

	"github.com/goccy/go-yaml"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
)

var yamlMapItemType = reflect.TypeOf(yaml.MapItem{})

// Execute executes templates of i with data.
func Execute(ctx *context.Context, data interface{}) (interface{}, error) {
	return ExecuteWithArgs(ctx, data, ctx)
}

func ExecuteWithArgs(ctx *context.Context, data, args interface{}) (interface{}, error) {
	v, err := execute(ctx, reflect.ValueOf(data), args)
	if err != nil {
		return nil, err
	}
	if v.IsValid() {
		return v.Interface(), nil
	}
	return nil, nil
}

func execute(ctx *context.Context, data reflect.Value, args interface{}) (reflect.Value, error) {
	v := reflectutil.Elem(data)
	switch v.Kind() {
	case reflect.Map:
		for _, k := range v.MapKeys() {
			e := v.MapIndex(k)
			if !isNil(e) {
				x, err := execute(ctx, e, args)
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
				x, err := execute(ctx, e, args)
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
				key := v.FieldByName("Key").Interface()
				ctx.AddChildPath(key.(string))
				x, err := execute(ctx, value, args)
				if yml, err := ctx.CurrentYAML(); err == nil {
					fmt.Printf("%s\n", yml)
				}
				if err != nil {
					return reflect.Value{}, err
				}
				value.Set(x)
			}
		default:
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				x, err := execute(ctx, field, args)
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
		x, err := tmpl.Execute(ctx, args)
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
