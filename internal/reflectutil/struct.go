package reflectutil

import (
	"reflect"
	"strings"
)

func StructFieldToKey(field reflect.StructField) string {
	for _, name := range []string{"yaml", "json"} {
		tag := field.Tag.Get(name)
		if tag != "" {
			tagValues := strings.Split(tag, ",")
			if len(tagValues) > 0 && tagValues[0] != "" {
				return tagValues[0]
			}
		}
	}
	return field.Name
}
