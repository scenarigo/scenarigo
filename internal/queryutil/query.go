package queryutil

import (
	"context"
	"reflect"
	"strings"
	"sync"

	query "github.com/zoncoen/query-go"
	yamlextractor "github.com/zoncoen/query-go/extractor/yaml"
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
				return e.v.Get(f).Interface(), true
			}
		}
		{
			name := f.TextName()
			if ci {
				name = strings.ToLower(name)
			}
			if name == key {
				return e.v.Get(f).Interface(), true
			}
		}
		if f.HasJSONName() {
			name := f.JSONName()
			if ci {
				name = strings.ToLower(name)
			}
			if name == key {
				return e.v.Get(f).Interface(), true
			}
		}
	}
	return nil, false
}

func AppendOptions(customOpts ...query.Option) {
	m.Lock()
	defer m.Unlock()
	opts = append(opts, customOpts...)
}
