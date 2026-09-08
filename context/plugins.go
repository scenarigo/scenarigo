package context

import (
	"context"

	query "github.com/zoncoen/query-go/v2"
)

// Plugins represents plugins.
type Plugins []map[string]any

var _ query.KeyExtractor = (Plugins)(nil)

// Append appends p to plugins.
func (plugins Plugins) Append(ps map[string]any) Plugins {
	if ps == nil {
		return plugins
	}
	plugins = append(plugins, ps)
	return plugins
}

// ExtractByKey implements query.KeyExtractor interface.
func (plugins Plugins) ExtractByKey(_ context.Context, key string) (any, error) {
	for _, ps := range plugins {
		if p, ok := ps[key]; ok {
			return p, nil
		}
	}
	return nil, query.ErrNotFound
}
