package context

// Plugins represents plugins.
type Plugins []map[string]any

// Append appends p to plugins.
func (plugins Plugins) Append(ps map[string]any) Plugins {
	if ps == nil {
		return plugins
	}
	plugins = append(plugins, ps)
	return plugins
}

// ExtractByKey implements query.KeyExtractor interface.
func (plugins Plugins) ExtractByKey(key string) (any, bool) {
	for _, ps := range plugins {
		if p, ok := ps[key]; ok {
			return p, true
		}
	}
	return nil, false
}
