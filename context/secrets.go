package context

import (
	"context"
	"errors"
	"fmt"
	"go/token"
	"reflect"
	"strings"

	query "github.com/zoncoen/query-go/v2"

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

var _ query.KeyExtractor = (*Secrets)(nil)

// ExtractByKey implements query.KeyExtractor interface.
func (s *Secrets) ExtractByKey(ctx context.Context, key string) (any, error) {
	// The context carries the options of the lookup this extractor is part of,
	// case-insensitivity included, so the sub-query behaves the same way.
	k := queryutil.NewFromContext(ctx).Key(key)
	for i := len(s.secrets) - 1; i >= 0; i-- {
		v, err := k.Extract(ctx, s.secrets[i])
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, query.ErrNotFound) {
			return nil, err
		}
	}
	return nil, query.ErrNotFound
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
		for i := range v.Len() {
			result = append(result, build(q.Index(i), v.Index(i))...)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			result = append(result, build(q.Key(fmt.Sprint(k.Interface())), v.MapIndex(k))...)
		}
	case reflect.Struct:
		for i := range v.NumField() {
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
