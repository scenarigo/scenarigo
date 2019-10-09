package context

import (
	"plugin"
	"reflect"
)

// Plugins represents plugins.
type Plugins []map[string]*plugin.Plugin

// Append appends p to plugins.
func (plugins Plugins) Append(ps map[string]*plugin.Plugin) Plugins {
	if ps == nil {
		return plugins
	}
	plugins = append(plugins, ps)
	return plugins
}

// ExtractByKey implements query.KeyExtractor interface.
func (plugins Plugins) ExtractByKey(key string) (interface{}, bool) {
	for _, ps := range plugins {
		ps := ps
		if p, ok := ps[key]; ok {
			return (*plug)(p), true
		}
	}
	return nil, false
}

type plug plugin.Plugin

// ExtractByKey implements query.KeyExtractor interface.
func (p *plug) ExtractByKey(key string) (interface{}, bool) {
	if sym, err := ((*plugin.Plugin)(p)).Lookup(key); err == nil {
		// sym is a pointer to a variable or function.
		return reflect.ValueOf(sym).Elem().Interface(), true
	}
	return nil, false
}
