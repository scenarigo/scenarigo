package wasm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/scenarigo/scenarigo/context"
)

// Value represents a return value from a WASM plugin function call.
type Value struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	Type  *Type  `json:"type"`
}

func EncodeValue(v reflect.Value) (*Value, error) {
	t, err := NewType(v)
	if err != nil {
		return nil, err
	}
	ret := &Value{
		ID:   fmt.Sprintf("%p", &v),
		Type: t,
	}
	switch t.Kind {
	case CONTEXT:
		if ctx, ok := v.Interface().(*context.Context); ok {
			sctx := ctx.ToSerializable()
			v = reflect.ValueOf(sctx)
		}
	case ERROR:
		if err, ok := v.Interface().(error); ok {
			ret.Value = err.Error()
			return ret, nil
		}
	}
	if t.Kind == FUNC || t.Kind == STRUCT || t.Kind == POINTER || t.Step || t.StepFunc || t.LeftArrowFunc {
		return ret, nil
	}

	// uintptr is not supported by the YAML marshaler.
	if t.Kind == UINTPTR {
		ret.Value = fmt.Sprint(v.Interface())
		return ret, nil
	}

	b, err := yaml.Marshal(v.Interface())
	if err != nil {
		return nil, fmt.Errorf("failed to encode value to yaml text: %w", err)
	}
	encoded, err := yaml.YAMLToJSON(b)
	if err != nil {
		return nil, fmt.Errorf("failed to encode to json value: %w", err)
	}
	ret.Value = strings.TrimSpace(string(encoded))
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
