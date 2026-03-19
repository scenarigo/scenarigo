package queryutil

import (
	"context"
	"reflect"
	"strings"
	"sync"

	query "github.com/zoncoen/query-go"
	yamlextractor "github.com/zoncoen/query-go/extractor/yaml"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

var (
	m    sync.RWMutex
	opts = []query.Option{}
)

func New(opts ...query.Option) *query.Query {
	return query.New(append(Options(), opts...)...)
}

func Options() []query.Option {
	m.RLock()
	defer m.RUnlock()
	return append(
		[]query.Option{
			query.ExtractByStructTag("yaml", "json"),
			query.CustomExtractFunc(yamlextractor.MapSliceExtractFunc()),
			query.CustomExtractFunc(dynamicpbExtractFunc()),
		},
		opts...,
	)
}

func dynamicpbExtractFunc() func(query.ExtractFunc) query.ExtractFunc {
	return func(f query.ExtractFunc) query.ExtractFunc {
		return func(in reflect.Value) (reflect.Value, bool) {
			v := in
			if v.IsValid() && v.CanInterface() {
				if msg, ok := v.Interface().(*dynamicpb.Message); ok {
					return f(reflect.ValueOf(&keyExtractor{
						v: msg,
					}))
				}
			}
			return f(in)
		}
	}
}

type keyExtractor struct {
	v *dynamicpb.Message
}

type ProtoEnum struct {
	number protoreflect.EnumNumber
	desc   protoreflect.EnumDescriptor
}

type ProtoEnumType struct {
	desc protoreflect.EnumDescriptor
}

func (t *ProtoEnumType) New(n protoreflect.EnumNumber) protoreflect.Enum {
	return nil
}

func (t *ProtoEnumType) Descriptor() protoreflect.EnumDescriptor {
	return t.desc
}

func (e *ProtoEnum) Type() protoreflect.EnumType {
	return &ProtoEnumType{desc: e.desc}
}

func (e *ProtoEnum) Number() protoreflect.EnumNumber {
	return e.number
}

func (e *ProtoEnum) ProtoReflect() protoreflect.Enum {
	return e
}

func (e *ProtoEnum) Descriptor() protoreflect.EnumDescriptor {
	return e.desc
}

// ExtractByKey implements the query.KeyExtractorContext interface.
func (e *keyExtractor) ExtractByKey(ctx context.Context, key string) (any, bool) {
	ci := query.IsCaseInsensitive(ctx)
	if ci {
		key = strings.ToLower(key)
	}
	fields := e.v.Descriptor().Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		{
			name := string(f.Name())
			if ci {
				name = strings.ToLower(name)
			}
			if name == key {
				return e.getField(f)
			}
		}
		{
			name := f.TextName()
			if ci {
				name = strings.ToLower(name)
			}
			if name == key {
				return e.getField(f)
			}
		}
		if f.HasJSONName() {
			name := f.JSONName()
			if ci {
				name = strings.ToLower(name)
			}
			if name == key {
				return e.getField(f)
			}
		}
	}
	return nil, false
}

func (e *keyExtractor) getField(f protoreflect.FieldDescriptor) (any, bool) {
	field := e.v.Get(f).Interface()
	if number, ok := field.(protoreflect.EnumNumber); ok {
		return &ProtoEnum{desc: f.Enum(), number: number}, true
	}
	return field, true
}

func AppendOptions(customOpts ...query.Option) {
	m.Lock()
	defer m.Unlock()
	opts = append(opts, customOpts...)
}
