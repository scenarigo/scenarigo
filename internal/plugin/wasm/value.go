package wasm

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/scenarigo/scenarigo/context"
)

// Value represents a return value from a WASM plugin function call.
type Value struct {
	ID              string `json:"id"`
	Value           string `json:"value"`
	IsStep          bool   `json:"isStep"`
	IsLeftArrowFunc bool   `json:"isLeftArrowFunc"`
}

func (v *Value) ToReflect() (reflect.Value, error) {
	return DecodeValue([]byte(v.Value))
}

func EncodeValue(v reflect.Value) (*Value, error) {
	t := v.Type()
	ret := &Value{}
	if t == ctxType {
		if ctx, ok := v.Interface().(*context.Context); ok {
			sctx := ctx.ToSerializable()
			v = reflect.ValueOf(sctx)
		}
	}
	var isImplementedCustomInterface bool
	if t == stepType || t.Implements(stepType) {
		ret.IsStep = true
		isImplementedCustomInterface = true
	}
	if t == leftArrowFuncType || t.Implements(leftArrowFuncType) {
		ret.IsLeftArrowFunc = true
		isImplementedCustomInterface = true
	}
	ret.ID = fmt.Sprintf("%p", &v)
	if !isImplementedCustomInterface {
		b, err := json.Marshal(v.Interface())
		if err != nil {
			return nil, fmt.Errorf("failed to encode %T value", v.Interface())
		}
		ret.Value = string(b)
	}
	return ret, nil
}

func DecodeValueWithType(t reflect.Type, data []byte) (reflect.Value, error) {
	if t == ctxType {
		var sctx context.SerializableContext
		if err := json.Unmarshal(data, &sctx); err != nil {
			return reflect.Value{}, err
		}
		ctx, err := context.FromSerializable(&sctx)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(ctx), nil
	}
	rv := reflect.New(t)
	v := rv.Interface()
	if err := json.Unmarshal(data, v); err != nil {
		return reflect.Value{}, fmt.Errorf("failed to decode %s value from %q", t, data)
	}
	return rv.Elem(), nil
}

func DecodeValue(data []byte) (reflect.Value, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return reflect.Value{}, fmt.Errorf("failed to decode value from %q", data)
	}
	return reflect.ValueOf(v), nil
}
