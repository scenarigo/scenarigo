package template

import "github.com/scenarigo/scenarigo/template/val"

var typeFunctions typeFunctionExtractor

type typeFunctionExtractor struct{}

func (m typeFunctionExtractor) ExtractByKey(key string) (any, bool) {
	if key == "type" {
		return func(in any) any {
			return val.NewValue(in).Type().Name()
		}, true
	}
	if t := val.GetType(key); t != nil {
		return func(in any) (any, error) {
			v, err := t.Convert(val.NewValue(in))
			if err != nil {
				return nil, err
			}
			return v.GoValue(), nil
		}, true
	}
	return nil, false
}
