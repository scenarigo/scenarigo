package wasm

import (
	"encoding/json"
	"fmt"
	"reflect"

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
	// Check for zero Value first to prevent panic in NewType
	if !v.IsValid() {
		return &Value{
			ID:    "nil",
			Value: "null\n",
			Type:  &Type{Kind: INVALID},
		}, nil
	}

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
	if t.Step || t.StepFunc || t.LeftArrowFunc {
		return ret, nil
	}

	// Special handling for function type - prevent YAML encoding errors
	if t.Kind == FUNC {
		// For function types, we don't encode the actual function value
		// The type information is sufficient for Host-Guest communication
		ret.Value = "null\n"
		return ret, nil
	}

	// Temporarily disable struct/pointer special handling to test if this is causing gRPC client issues
	// TODO: Re-enable after identifying the root cause
	/*
		// Special handling for structs/pointers that might cause YAML encoding errors
		if t.Kind == STRUCT || t.Kind == POINTER {
			// Try YAML marshaling first, fallback to null if it fails or results in empty content
			if b, err := yaml.Marshal(v.Interface()); err == nil {
				// YAML marshaling succeeded, check if the result is meaningful
				trimmed := strings.TrimSpace(string(b))
				if trimmed == "{}" || trimmed == "" || trimmed == "null" {
					// Empty or meaningless YAML content, treat as reference-only struct
					ret.Value = "null\n"
					return ret, nil
				}

				// YAML to JSON conversion
				encoded, err := yaml.YAMLToJSON(b)
				if err != nil {
					// YAML to JSON conversion failed, use null
					ret.Value = "null\n"
					return ret, nil
				}
				ret.Value = string(encoded)
				return ret, nil
			} else {
				// YAML marshaling failed, return null to avoid errors
				ret.Value = "null\n"
				return ret, nil
			}
		}
	*/

	// Special handling for uintptr type
	if t.Kind == UINTPTR {
		if ptr, ok := v.Interface().(uintptr); ok {
			// Convert uintptr to uint64 for JSON encoding
			ret.Value = fmt.Sprintf("%d\n", uint64(ptr))
			return ret, nil
		}
	}

	// Handle interface types by checking the actual value they contain
	if t.Kind == ANY {
		if !v.IsNil() {
			// Get the actual value contained in the interface
			actualValue := v.Elem()
			actualKind := actualValue.Kind()

			// If the actual value is a function, handle it specially
			if actualKind == reflect.Func {
				ret.Value = "null\n"
				return ret, nil
			}

			// If the actual value is uintptr, handle it specially
			if actualKind == reflect.Uintptr {
				if ptr, ok := actualValue.Interface().(uintptr); ok {
					ret.Value = fmt.Sprintf("%d\n", uint64(ptr))
					return ret, nil
				}
			}

			// For complex types, don't try to recursively encode them
			// Just return null for complex types when they're inside interfaces
			if actualKind == reflect.Struct || actualKind == reflect.Map ||
				actualKind == reflect.Slice || actualKind == reflect.Array ||
				actualKind == reflect.Ptr {
				ret.Value = "null\n"
				return ret, nil
			}
		}
	}

	// normalize yaml.MapSlice or yaml.MapItem value.
	b, err := yaml.Marshal(v.Interface())
	if err != nil {
		return nil, fmt.Errorf("failed to encode value to yaml text: %w", err)
	}
	encoded, err := yaml.YAMLToJSON(b)
	if err != nil {
		return nil, fmt.Errorf("failed to encode to json value: %w", err)
	}
	ret.Value = string(encoded)
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

	// Special handling for uintptr type
	if t.Kind() == reflect.Uintptr {
		var val uint64
		if err := json.Unmarshal(data, &val); err != nil {
			return reflect.Value{}, fmt.Errorf("failed to decode uintptr value from %q", data)
		}
		return reflect.ValueOf(uintptr(val)), nil
	}

	rv := reflect.New(t)
	v := rv.Interface()
	if err := json.Unmarshal(data, v); err != nil {
		return reflect.Value{}, fmt.Errorf("failed to decode %s value from %q", t, data)
	}
	return rv.Elem(), nil
}
