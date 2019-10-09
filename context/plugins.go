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
		// If sym is a pointer to a variable, return the actual variable for convenience.
		if v := reflect.ValueOf(sym); v.Kind() == reflect.Ptr {
			return v.Elem().Interface(), true
		}
		return sym, true
	}
	return nil, false
}
