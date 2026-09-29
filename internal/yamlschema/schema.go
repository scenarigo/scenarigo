package yamlschema

import "unicode"

// FieldInfo describes a YAML field in a schema.
type FieldInfo struct {
	// Name is the YAML key name.
	Name string
	// Type describes the field type.
	Type FieldType
	// Description is a human-readable description.
	Description string
	// EnumValues lists allowed values (if any).
	EnumValues []string
	// OpenEnum marks EnumValues as the built-in values only: other values
	// are legal because plugins can register more (for example protocols).
	OpenEnum bool
	// Children lists child fields (for object types).
	Children []*FieldInfo
	// Deprecated marks the field as deprecated.
	Deprecated bool
	// Required marks the field as required.
	Required bool
	// IsFilePath indicates the field value is a file path and should offer
	// filesystem-based completion.
	IsFilePath bool
	// DynamicKey is the sibling field name whose value determines dynamic children.
	// For example, "protocol" means the value of the "protocol" sibling field is used.
	DynamicKey string
	// DynamicChildren resolves children based on a discriminator value.
	// For example, "request" children depend on "protocol" value.
	DynamicChildren func(discriminator string) []*FieldInfo
}

type FieldType int

const (
	FieldTypeString FieldType = iota
	FieldTypeInt
	FieldTypeFloat
	FieldTypeBool
	FieldTypeObject
	FieldTypeArray
	FieldTypeMap
	FieldTypeAny
	FieldTypeDuration
)

func (t FieldType) String() string {
	switch t {
	case FieldTypeString:
		return "string"
	case FieldTypeInt:
		return "int"
	case FieldTypeFloat:
		return "float"
	case FieldTypeBool:
		return "bool"
	case FieldTypeObject:
		return "object"
	case FieldTypeArray:
		return "array"
	case FieldTypeMap:
		return "map"
	case FieldTypeAny:
		return "any"
	case FieldTypeDuration:
		return "duration"
	default:
		return "unknown"
	}
}

// Schema holds the complete schema for a document type.
type Schema struct {
	Fields []*FieldInfo
}

// FindField looks up a field by navigating the given key path.
// Numeric path segments (e.g., "0") are treated as array indices and skipped,
// continuing with the same field set (array items share the parent's children).
func (s *Schema) FindField(path []string) *FieldInfo {
	if s == nil || len(path) == 0 {
		return nil
	}
	fields := s.Fields
	var found *FieldInfo
	mapKey := false
	for _, key := range path {
		if mapKey {
			// The key of a map field is user-defined and has no schema entry.
			mapKey = false
			found = nil
			continue
		}
		if isNumeric(key) {
			continue
		}
		found = nil
		for _, f := range fields {
			if f.Name == key {
				found = f
				fields = f.children("")
				mapKey = f.hasMapChildren()
				break
			}
		}
		if found == nil {
			return nil
		}
	}
	return found
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// ChildFields returns the child fields at the given path.
// If context (sibling field values) is provided, it may resolve dynamic children.
// The key of a map field with children is user-defined and is skipped.
func (s *Schema) ChildFields(path []string, context map[string]string) []*FieldInfo {
	if s == nil {
		return nil
	}
	fields := s.Fields
	mapKey := false
	for _, key := range path {
		if mapKey {
			mapKey = false
			continue
		}
		var current *FieldInfo
		for _, f := range fields {
			if f.Name == key {
				current = f
				break
			}
		}
		if current == nil {
			return nil
		}
		discriminator := ""
		if context != nil && current.DynamicKey != "" {
			discriminator = context[current.DynamicKey]
		}
		fields = current.children(discriminator)
		mapKey = current.hasMapChildren()
	}
	return fields
}

// children resolves the child fields, using the discriminator for dynamic children.
func (f *FieldInfo) children(discriminator string) []*FieldInfo {
	if f.DynamicChildren != nil {
		return f.DynamicChildren(discriminator)
	}
	return f.Children
}

// hasMapChildren reports whether f is a map whose values have a schema of their own.
func (f *FieldInfo) hasMapChildren() bool {
	return f.Type == FieldTypeMap && len(f.Children) > 0
}
