package queryutil

import (
	"context"
	"reflect"
	"strings"
	"sync"

	yamlextractor "github.com/zoncoen/query-go/extractor/yaml/v2"
	query "github.com/zoncoen/query-go/v2"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

var (
	m    sync.RWMutex
	opts = []query.Option{}
)

func New(opts ...query.Option) *query.Query {
	return query.New(optionsWith(opts)...)
}

// Options returns the process-wide query options: the base set, what the
// registered protocols added, and the adapter for v1-shaped extractors last.
func Options() []query.Option {
	return optionsWith(nil)
}

// optionsWith returns the process-wide options with extra inserted just before
// the adapter for v1-shaped extractors, which has to stay last.
//
// query-go composes custom extract funcs so that the first is the outermost,
// so last means innermost: the adapter hands its stand-in to the extraction
// itself and to nothing else. Anywhere earlier, the funcs a protocol or a
// caller registers would be handed the stand-in instead of the value it
// adapts - they could not recognise a concrete type that also has a v1
// extractor shape, and what they did with it would decide whether the lookup
// falls back to reflecting over the original value. Innermost also matches
// what those funcs saw before there was an adapter at all.
func optionsWith(extra []query.Option) []query.Option {
	m.RLock()
	defer m.RUnlock()
	all := make([]query.Option, 0, len(opts)+len(extra)+4)
	all = append(all,
		query.ExtractByStructTag("yaml", "json"),
		query.CustomExtractFunc(yamlextractor.MapSliceExtractFunc()),
		query.CustomExtractFunc(dynamicpbExtractFunc()),
	)
	all = append(all, opts...)
	all = append(all, extra...)
	return append(all, query.CustomExtractFunc(legacyExtractFunc()))
}

func dynamicpbExtractFunc() func(query.ExtractFunc) query.ExtractFunc {
	return func(f query.ExtractFunc) query.ExtractFunc {
		return func(ctx context.Context, in reflect.Value) (reflect.Value, error) {
			v := in
			if v.IsValid() && v.CanInterface() {
				if msg, ok := v.Interface().(*dynamicpb.Message); ok {
					return f(ctx, reflect.ValueOf(&keyExtractor{
						v: msg,
					}))
				}
			}
			return f(ctx, in)
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

var _ query.KeyExtractor = (*keyExtractor)(nil)

// ExtractByKey implements the query.KeyExtractor interface.
func (e *keyExtractor) ExtractByKey(ctx context.Context, key string) (any, error) {
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
	return nil, query.ErrNotFound
}

func (e *keyExtractor) getField(f protoreflect.FieldDescriptor) (any, error) {
	field := e.v.Get(f).Interface()
	if number, ok := field.(protoreflect.EnumNumber); ok {
		return &ProtoEnum{desc: f.Enum(), number: number}, nil
	}
	return field, nil
}

func AppendOptions(customOpts ...query.Option) {
	m.Lock()
	defer m.Unlock()
	opts = append(opts, customOpts...)
}
