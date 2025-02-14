package reflectutil

import (
	"reflect"
	"testing"
)

func TestStructFieldToKey(t *testing.T) {
	v := reflect.ValueOf(testStruct{})
	tests := map[string]struct {
		field  reflect.StructField
		expect string
	}{
		"yaml": {
			field:  getField(t, v, "YAML"),
			expect: "yamltag",
		},
		"json": {
			field:  getField(t, v, "JSON"),
			expect: "jsontag",
		},
		"struct name": {
			field:  getField(t, v, "Name"),
			expect: "Name",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := StructFieldToKey(test.field)
			if got != test.expect {
				t.Fatalf("expect %q but got %q", test.expect, got)
			}
		})
	}
}

func getField(t *testing.T, v reflect.Value, name string) reflect.StructField {
	t.Helper()
	f, ok := v.Type().FieldByName(name)
	if !ok {
		t.Fatalf("field %q not found", name)
	}
	return f
}

//nolint:govet
type testStruct struct {
	YAML string `json:"jsontag" yaml:"yamltag"`
	JSON string `json:"jsontag"`
	Name string
}
