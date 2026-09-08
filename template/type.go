package template

import (
	"context"

	query "github.com/zoncoen/query-go/v2"

	"github.com/scenarigo/scenarigo/template/val"
)

var typeFunctions typeFunctionExtractor

type typeFunctionExtractor struct{}

var _ query.KeyExtractor = typeFunctionExtractor{}

// ExtractByKey implements query.KeyExtractor interface.
func (m typeFunctionExtractor) ExtractByKey(_ context.Context, key string) (any, error) {
	if key == "type" {
		return func(in any) any {
			return val.NewValue(in).Type().Name()
		}, nil
	}
	if t := val.GetType(key); t != nil {
		return func(in any) (any, error) {
			v, err := t.Convert(val.NewValue(in))
			if err != nil {
				return nil, err
			}
			return v.GoValue(), nil
		}, nil
	}
	return nil, query.ErrNotFound
}
