package yamlschema

import (
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/protocol/grpc"
	"github.com/scenarigo/scenarigo/protocol/http"
	"github.com/scenarigo/scenarigo/schema"
)

// TestSchemaMatchesGoStructs guards against drift between the hand-written
// field schema and the Go structs that actually unmarshal the YAML files.
// Every YAML-tagged field of the Go struct must have a schema entry, and
// every schema entry must correspond to a Go field.
func TestSchemaMatchesGoStructs(t *testing.T) {
	configFields := ConfigSchema().Fields
	tests := []struct {
		name   string
		fields []*FieldInfo
		typ    any
	}{
		{name: "config", fields: configFields, typ: schema.Config{}},
		{name: "config.plugins.*", fields: mustFind(t, configFields, "plugins").Children, typ: schema.PluginConfig{}},
		{name: "config.protocols.grpc", fields: mustFind(t, mustFind(t, configFields, "protocols").Children, "grpc").Children, typ: grpc.Option{}},
		{name: "scenario", fields: ScenarioSchema().Fields, typ: schema.Scenario{}},
		{name: "scenario.steps[]", fields: stepFields(), typ: schema.Step{}},
		{name: "step.request(http)", fields: httpRequestFields(), typ: http.Request{}},
		{name: "step.expect(http)", fields: httpExpectFields(), typ: http.Expect{}},
		{name: "step.request(grpc)", fields: grpcRequestFields(), typ: grpc.Request{}},
		{name: "step.expect(grpc)", fields: grpcExpectFields(), typ: grpc.Expect{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compareFields(t, tt.name, tt.fields, reflect.TypeOf(tt.typ))
		})
	}
}

func mustFind(t *testing.T, fields []*FieldInfo, name string) *FieldInfo {
	t.Helper()
	for _, f := range fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("schema field %q not found", name)
	return nil
}

func compareFields(t *testing.T, path string, fields []*FieldInfo, typ reflect.Type) {
	t.Helper()
	goFields := yamlFields(typ)
	byName := make(map[string]*FieldInfo, len(fields))
	for _, f := range fields {
		byName[f.Name] = f
	}
	for name, sf := range goFields {
		f, ok := byName[name]
		if !ok {
			t.Errorf("%s.%s exists in %s but not in the schema", path, name, typ)
			continue
		}
		if f.DynamicChildren != nil {
			continue // protocol-dependent children are compared separately
		}
		if child := structType(sf.Type); child != nil {
			compareFields(t, path+"."+name, f.Children, child)
		}
	}
	for _, f := range fields {
		if _, ok := goFields[f.Name]; !ok {
			t.Errorf("schema field %s.%s does not exist in %s", path, f.Name, typ)
		}
	}
}

// yamlFields returns the YAML-tagged fields of a struct type keyed by YAML name.
func yamlFields(typ reflect.Type) map[string]reflect.StructField {
	out := map[string]reflect.StructField{}
	for i := range typ.NumField() {
		sf := typ.Field(i)
		if !sf.IsExported() {
			continue
		}
		if sf.Anonymous {
			if child := structType(sf.Type); child != nil {
				maps.Copy(out, yamlFields(child))
			}
			continue
		}
		tag, ok := sf.Tag.Lookup("yaml")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(sf.Name)
		}
		out[name] = sf
	}
	return out
}

// structType returns the struct type reachable through pointers and slices
// when it declares YAML-tagged fields of its own, and nil otherwise.
func structType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	if len(yamlFields(typ)) == 0 {
		return nil
	}
	return typ
}
