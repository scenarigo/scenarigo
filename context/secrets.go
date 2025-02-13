package context

import (
	"context"
	"fmt"
	"go/token"
	"reflect"
	"strings"

	"github.com/zoncoen/query-go"

	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

// Secrets represents context secrets.
type Secrets struct {
	secrets []any
	values  []queryValue
}

type queryValue struct {
	query string
	v     string
}

// Append appends v to context secrets.
func (s *Secrets) Append(v any) *Secrets {
	if v == nil {
		return s
	}
	if s == nil {
		s = &Secrets{}
	}
	sl := make([]any, 0, len(s.secrets)+1)
	sl = append(sl, s.secrets...)
	sl = append(sl, v)
	vs := append(make([]queryValue, 0, len(s.values)), s.values...)
	vs = append(vs, build(query.New().Key("secrets"), reflect.ValueOf(v))...)
	return &Secrets{
		secrets: sl,
		values:  vs,
	}
}

// ExtractByKey implements query.KeyExtractorContext interface.
func (s *Secrets) ExtractByKey(ctx context.Context, key string) (any, bool) {
	var opts []query.Option
	if query.IsCaseInsensitive(ctx) {
		opts = append(opts, query.CaseInsensitive())
	}
	k := queryutil.New(opts...).Key(key)
	for i := len(s.secrets) - 1; i >= 0; i-- {
		if v, err := k.Extract(s.secrets[i]); err == nil {
			return v, true
		}
	}
	return nil, false
}

func (s *Secrets) ReplaceAll(str string) string {
	for _, v := range s.values {
		str = strings.ReplaceAll(str, v.v, fmt.Sprintf("{{%s}}", v.query))
	}
	return str
}

func build(q *query.Query, in reflect.Value) []queryValue {
	v := reflectutil.Elem(in)
	var result []queryValue
	switch v.Kind() {
	case reflect.Invalid:
		return nil
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			result = append(result, build(q.Index(i), v.Index(i))...)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			result = append(result, build(q.Key(fmt.Sprint(k.Interface())), v.MapIndex(k))...)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			ft := v.Type().Field(i)
			if !token.IsExported(ft.Name) {
				continue // skip unexported field
			}
			result = append(result, build(q.Key(reflectutil.StructFieldToKey(ft)), v.Field(i))...)
		}
	default:
		return []queryValue{
			{
				query: strings.TrimPrefix(q.String(), "."),
				v:     fmt.Sprint(v),
			},
		}
	}
	return result
}
