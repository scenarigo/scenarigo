package context

import "github.com/scenarigo/scenarigo/internal/queryutil"

// Vars represents context variables.
type Vars []any

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
func (vars Vars) ExtractByKey(key string) (any, bool) {
	k := queryutil.New().Key(key)
	for i := len(vars) - 1; i >= 0; i-- {
		if v, err := k.Extract(vars[i]); err == nil {
			return v, true
		}
	}
	return nil, false
}
