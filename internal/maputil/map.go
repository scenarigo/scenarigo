package maputil

import (
	"reflect"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
)

// ConvertStringsMapSlice convert yaml.MapSlice( map[string]interface{} ) to yaml.MapSlice( map[string][]string ).
func ConvertStringsMapSlice(in yaml.MapSlice) (yaml.MapSlice, error) {
	m := map[string]interface{}{}
	for _, v := range in {
		key, ok := v.Key.(string)
		if !ok {
			return nil, errors.Errorf("map key type must be string. but %T", v.Key)
		}
		m[key] = v.Value
	}
	out := make(yaml.MapSlice, 0, len(in))
	convertedMap, err := reflectutil.ConvertStringsMap(reflect.ValueOf(m))
	if err != nil {
		return nil, err
	}
	for _, item := range in {
		out = append(out, yaml.MapItem{
			Key:   item.Key,
			Value: convertedMap[item.Key.(string)],
		})
	}
	return out, nil
}
