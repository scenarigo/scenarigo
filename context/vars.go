package context

import (
	"context"
	"errors"

	query "github.com/zoncoen/query-go/v2"

	"github.com/scenarigo/scenarigo/internal/queryutil"
)

// Vars represents context variables.
type Vars []any

var _ query.KeyExtractor = (Vars)(nil)

// Append appends v to context variables.
func (vars Vars) Append(v any) Vars {
	if v == nil {
		return vars
	}
	sl := make([]any, 0, len(vars)+1)
	sl = append(sl, vars...)
	sl = append(sl, v)
	return sl
}

// ExtractByKey implements query.KeyExtractor interface.
func (vars Vars) ExtractByKey(ctx context.Context, key string) (any, error) {
	k := queryutil.NewFromContext(ctx).Key(key)
	for i := len(vars) - 1; i >= 0; i-- {
		v, err := k.Extract(ctx, vars[i])
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, query.ErrNotFound) {
			return nil, err
		}
	}
	return nil, query.ErrNotFound
}
