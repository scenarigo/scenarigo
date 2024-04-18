package template

import (
	"context"
	"fmt"
	"go/token"
	"reflect"

	"github.com/goccy/go-yaml"

	"github.com/zoncoen/scenarigo/errors"
	"github.com/zoncoen/scenarigo/internal/queryutil"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
)

//nolint:exhaustruct
var (
	yamlMapItemType = reflect.TypeOf(yaml.MapItem{})
	funcCallType    = reflect.TypeOf(FuncCall{})
)

// Execute executes templates of i with data.
func Execute(ctx context.Context, i, data interface{}) (interface{}, error) {
	v, err := execute(ctx, reflect.ValueOf(i), data)
	if err != nil {
		return nil, err
	}
	if v.IsValid() {
		return v.Interface(), nil
	}
	return nil, nil //nolint:nilnil
}

//nolint:gocyclo,cyclop,maintidx
func execute(ctx context.Context, in reflect.Value, data interface{}) (reflect.Value, error) {
	v := reflectutil.Elem(in)
	switch v.Kind() {
	case reflect.Invalid:
		return in, nil
	case reflect.Map:
		for _, k := range v.MapKeys() {
			e := v.MapIndex(k)
			if !isNil(e) {
				keyStr := fmt.Sprintf(".'%s'", k.Interface())
				key, err := execute(ctx, k, data)
				if err != nil {
					return reflect.Value{}, err
				}
				// left arrow function
				if ke := reflectutil.Elem(key); ke.IsValid() && ke.Type() == funcCallType && ke.CanInterface() {
					if len(v.MapKeys()) != 1 {
						return reflect.Value{}, errors.New("invalid left arrow function call")
					}
					f := ke.Interface().(FuncCall) //nolint:forcetypeassert
					res, err := executeLeftArrowFunction(ctx, f.Func, e, data, keyStr)
					if err != nil {
						return reflect.Value{}, errors.WithPath(fmt.Errorf("failed to execute left arrow function: %w", err), keyStr)
					}
					v = res
					break
				}
				x, err := convert(e.Type())(execute(ctx, e, data))
				if err != nil {
					return reflect.Value{}, errors.WithPath(err, keyStr)
				}
				v.SetMapIndex(k, reflect.Value{}) // delete old value
				v.SetMapIndex(key, x)
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			e := v.Index(i)
			if !isNil(e) {
				if !e.CanSet() {
					e = makePtr(e).Elem() // create pointer to enable to set values
				}
				if e.Type() == yamlMapItemType {
					key := e.FieldByName("Key")
					keyStr := fmt.Sprintf(".'%s'", key.Interface())
					value := e.FieldByName("Value")
					if !isNil(key) {
						k, err := execute(ctx, key, data)
						if err != nil {
							return reflect.Value{}, err
						}
						key.Set(k)
					}
					// left arrow function
					if ke := reflectutil.Elem(key); ke.IsValid() && ke.Type() == funcCallType && ke.CanInterface() {
						if v.Len() != 1 {
							return reflect.Value{}, errors.New("invalid left arrow function call")
						}
						f := ke.Interface().(FuncCall) //nolint:forcetypeassert
						res, err := executeLeftArrowFunction(ctx, f.Func, value, data, keyStr)
						if err != nil {
							return reflect.Value{}, errors.WithPath(fmt.Errorf("failed to execute left arrow function: %w", err), keyStr)
						}
						v = res
						break
					}
					val, err := convert(value.Type())(execute(ctx, value, data))
					if err != nil {
						return reflect.Value{}, errors.WithPath(err, keyStr)
					}
					value.Set(val)
					continue
				}
				x, err := convert(e.Type())(execute(ctx, e, data))
				if err != nil {
					return reflect.Value{}, errors.WithQuery(err, queryutil.New().Index(i))
				}
				e.Set(x)
			}
		}
	case reflect.Struct:
		if !v.CanSet() {
			v = makePtr(v).Elem() // create pointer to enable to set values
		}
		for i := 0; i < v.NumField(); i++ {
			if !token.IsExported(v.Type().Field(i).Name) {
				continue // skip unexported field
			}
			field := v.Field(i)
			x, err := convert(field.Type())(execute(ctx, field, data))
			if err != nil {
				fieldName := reflectutil.StructFieldToKey(v.Type().Field(i))
				return reflect.Value{}, errors.WithPath(err, fieldName)
			}
			if err := reflectutil.Set(field, x); err != nil {
				fieldName := reflectutil.StructFieldToKey(v.Type().Field(i))
				return reflect.Value{}, errors.WithPath(err, fieldName)
			}
		}
	case reflect.String:
		tmpl, err := New(v.String())
		if err != nil {
			return reflect.Value{}, err
		}
		x, err := tmpl.Execute(ctx, data)
		if err != nil {
			return reflect.Value{}, err
		}
		v = reflect.ValueOf(x)
	default:
	}

	// keep the original type as much as possible
	if in.IsValid() && v.IsValid() {
		if converted, err := convert(in.Type())(v, nil); err == nil {
			v = converted
		}
		// keep the original address
		if in.Type().Kind() == reflect.Ptr && v.Type().Kind() == reflect.Ptr {
			if v.Elem().Type().AssignableTo(in.Elem().Type()) {
				if err := reflectutil.Set(in.Elem(), v.Elem()); err != nil {
					return reflect.Value{}, err
				}
				v = in
			}
		}
	}
	return v, nil
}

func executeLeftArrowFunction(ctx context.Context, f Func, v reflect.Value, data any, str string) (reflect.Value, error) {
	if !isNil(v) {
		x, err := execute(ctx, v, data)
		if err != nil {
			return reflect.Value{}, err
		}
		v = x
	}
	s := new(funcStash)
	x, err := replaceFuncs(v, s)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("failed to stash functions to marshal argument to YAML: %w", err)
	}
	b, err := yaml.Marshal(x.Interface())
	if err != nil {
		return reflect.Value{}, fmt.Errorf("failed to marshal argument to YAML: %w", err)
	}
	arg, err := f.UnmarshalArg(func(v interface{}) error {
		if err := yaml.UnmarshalWithOptions(b, v, yaml.UseOrderedMap(), yaml.Strict()); err != nil {
			return err
		}

		// Restore functions that are replaced into strings.
		// See the "HACK" comment of *Template.executeParameterExpr method.
		arg, err := Execute(ctx, v, s)
		if err != nil {
			return fmt.Errorf("failed to restore functions: %w", err)
		}
		// NOTE: Decode method ensures that v is a pointer.
		rv := reflect.ValueOf(v).Elem()
		ev, err := convert(rv.Type())(reflect.ValueOf(arg), nil)
		if err != nil {
			return err
		}
		rv.Set(ev)

		return nil
	})
	if err != nil {
		return reflect.Value{}, fmt.Errorf("failed to unmarshal argument: %w", err)
	}
	res, err := f.Exec(arg)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("failed to execute function: %w", err)
	}

	// HACK: return error with path
	if str != "" {
		if as, ok := res.(interface {
			Assert(v interface{}) error
		}); ok {
			res = assertionFunc(func(v any) error {
				if err := as.Assert(v); err != nil {
					return errors.WithPath(err, str)
				}
				return nil
			})
		}
	}

	return reflect.ValueOf(res), nil
}

// NOTE: This function must return a copy to avoid the not found error when retrying.
func replaceFuncs(in reflect.Value, s *funcStash) (reflect.Value, error) {
	v := reflectutil.Elem(in)

	switch v.Kind() {
	case reflect.Map:
		vv := reflect.MakeMapWithSize(v.Type(), v.Len())
		for _, k := range v.MapKeys() {
			e := v.MapIndex(k)
			if !isNil(e) {
				x, err := replaceFuncs(e, s)
				if err != nil {
					return reflect.Value{}, err
				}
				vv.SetMapIndex(k, x)
			}
		}
		v = vv
	case reflect.Slice:
		vv := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			e := v.Index(i)
			if !isNil(e) {
				x, err := replaceFuncs(e, s)
				if err != nil {
					return reflect.Value{}, err
				}
				vv.Index(i).Set(x)
			}
		}
		v = vv
	case reflect.Struct:
		vv := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			if !token.IsExported(v.Type().Field(i).Name) {
				continue // skip unexported field
			}
			field := v.Field(i)
			x, err := replaceFuncs(field, s)
			if err != nil {
				return reflect.Value{}, err
			}
			if err := reflectutil.Set(vv.Field(i), x); err != nil {
				fieldName := reflectutil.StructFieldToKey(vv.Type().Field(i))
				return reflect.Value{}, errors.WithPath(err, fieldName)
			}
		}
		v = vv
	case reflect.Func:
		return reflect.ValueOf(fmt.Sprintf("{{%s}}", s.save(v.Interface()))), nil
	default:
		return in, nil
	}

	// keep the original type as much as possible
	if in.IsValid() && v.IsValid() {
		if converted, err := convert(in.Type())(v, nil); err == nil {
			v = converted
		}
	}
	return v, nil
}

func isNil(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// convert returns a function that converts a value to the given type t.
func convert(t reflect.Type) func(reflect.Value, error) (reflect.Value, error) {
	return func(v reflect.Value, err error) (reflect.Value, error) {
		if err != nil {
			return v, err
		}
		vv, _, err := reflectutil.Convert(t, v)
		if err != nil {
			return v, err
		}
		return vv, nil
	}
}

func makePtr(v reflect.Value) reflect.Value {
	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)
	return ptr
}

type assertionFunc func(v interface{}) error

func (f assertionFunc) Assert(v interface{}) error {
	return f(v)
}
