package reflectutil

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
)

// Set assigns v to the value target.
func Set(target, v reflect.Value) (retErr error) {
	if !target.IsValid() {
		return errors.New("can not set to invalid value")
	}
	if !v.IsValid() {
		return nil
	}
	defer func() {
		if err := recover(); err != nil {
			retErr = fmt.Errorf("can not set %s to %s: %s", v.Type().String(), target.Type().String(), err)
		}
	}()
	if vv, ok, err := Convert(target.Type(), v); err == nil && ok {
		v = vv
	}
	if !target.CanSet() {
		if !target.CanAddr() {
			return fmt.Errorf("can not set to unaddressable value")
		}
		return fmt.Errorf("can not set to unexported struct field")
	}
	if !v.Type().AssignableTo(target.Type()) {
		return fmt.Errorf("%s is not assignable to %s", v.Type().String(), target.Type().String())
	}
	target.Set(v)
	return nil
}

// Convert returns the value v converted to type t.
func Convert(t reflect.Type, v reflect.Value) (_ reflect.Value, _ bool, retErr error) {
	defer func() {
		if err := recover(); err != nil {
			retErr = errors.Errorf("failed to convert %T to %s: %s", v.Interface(), t.Name(), err)
		}
	}()

	if t == nil {
		return v, false, errors.New("failed to convert to untyped nil")
	}

	if !v.IsValid() {
		switch t.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			zero := reflect.New(t).Elem()
			return zero, true, nil
		default:
			return v, false, nil
		}
	}

	if convertibleTo(v.Type(), t) {
		return v.Convert(t), true, nil
	}
	if v.Type().Kind() == reflect.Ptr {
		if e := v.Elem(); e.IsValid() {
			if convertibleTo(e.Type(), t) {
				return v.Elem().Convert(t), true, nil
			}
		}
	} else {
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		if convertibleTo(ptr.Type(), t) {
			return ptr.Convert(t), true, nil
		}
	}

	return v, false, nil
}

func convertibleTo(src reflect.Type, dst reflect.Type) bool {
	if dst.Kind() == reflect.String {
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return false
		}
	}
	return src.ConvertibleTo(dst)
}

// ConvertInterface returns the value v converted to type t.
func ConvertInterface(t reflect.Type, v any) (any, bool, error) {
	vv, ok, err := Convert(t, reflect.ValueOf(v))
	if err != nil {
		return vv, ok, err
	}
	if ok {
		if vv.IsValid() && vv.CanInterface() {
			return vv.Interface(), ok, nil
		}
	}
	return v, ok, nil
}
