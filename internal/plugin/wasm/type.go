package wasm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/schema"
	"github.com/scenarigo/scenarigo/template"
)

var cacheTypeMap = map[string]*Type{}

// NameWithType represents a named type from a WASM plugin.
type NameWithType struct {
	Name string `json:"name"`
	Type *Type  `json:"type"`
}

// Type represents a type information from a WASM plugin.
type Type struct {
	Kind          Kind         `json:"kind"`
	MethodNames   []string     `json:"methodNames"`
	Step          bool         `json:"step"`
	StepFunc      bool         `json:"stepFunc"`
	LeftArrowFunc bool         `json:"leftArrowFunc"`
	Ref           string       `json:"ref"`
	Any           *AnyType     `json:"any"`
	Pointer       *PointerType `json:"pointer"`
	Struct        *StructType  `json:"struct"`
	Slice         *SliceType   `json:"slice"`
	Array         *ArrayType   `json:"array"`
	Map           *MapType     `json:"map"`
	Func          *FuncType    `json:"func"`
}

type Kind string

const (
	INVALID    Kind = "invalid"
	REF        Kind = "ref"
	INT        Kind = "int"
	INT8       Kind = "int8"
	INT16      Kind = "int16"
	INT32      Kind = "int32"
	INT64      Kind = "int64"
	UINT       Kind = "uint"
	UINT8      Kind = "uint8"
	UINT16     Kind = "uint16"
	UINT32     Kind = "uint32"
	UINT64     Kind = "uint64"
	UINTPTR    Kind = "uintptr"
	FLOAT32    Kind = "float32"
	FLOAT64    Kind = "float64"
	STRING     Kind = "string"
	BYTES      Kind = "bytes"
	BOOL       Kind = "bool"
	STRUCT     Kind = "struct"
	SLICE      Kind = "slice"
	ARRAY      Kind = "array"
	MAP        Kind = "map"
	FUNC       Kind = "func"
	POINTER    Kind = "pointer"
	ANY        Kind = "any"
	ERROR      Kind = "error"
	CONTEXT    Kind = "context"
	SCHEMASTEP Kind = "schema.step"
)

const nullString = "null"

var (
	errorType = reflect.TypeOf((*error)(nil)).Elem()
	stepType  = reflect.TypeOf((*interface {
		Run(*context.Context, *schema.Step) *context.Context
	})(nil)).Elem()
	leftArrowFuncType = reflect.TypeOf((*template.Func)(nil)).Elem()
	ctxType           = reflect.TypeOf((*context.Context)(nil))
	schemaStepType    = reflect.TypeOf((*schema.Step)(nil))
)

// PointerTyep represents pointer type information from a WASM plugin.
type PointerType struct {
	Elem *Type `json:"elem"`
}

// AnyType represents any type information from a WASM plugin.
type AnyType struct {
	Elem *Type `json:"elem"`
}

// FuncType represents function type information from a WASM plugin.
type FuncType struct {
	Args   []*Type `json:"args"`
	Return []*Type `json:"return"`
}

// SliceType represents slice type information from a WASM plugin.
type SliceType struct {
	Elem *Type `json:"elem"`
}

// ArrayType represents array type information from a WASM plugin.
type ArrayType struct {
	Num  int   `json:"num"`
	Elem *Type `json:"elem"`
}

// StructType represents struct type information from a WASM plugin.
type StructType struct {
	Fields []*NameWithType `json:"fields"`
}

func (t *StructType) HasField(name string) bool {
	for _, field := range t.Fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

// MapType represents map type information from a WASM plugin.
type MapType struct {
	Key   *Type `json:"key"`
	Value *Type `json:"value"`
}

type Any struct {
	Elem any
}

func (a *Any) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	a.Elem = v
	return nil
}

func (a *Any) String() string {
	return fmt.Sprint(a.Elem)
}

func toTypeID(t reflect.Type) string {
	return fmt.Sprintf("%p", t)
}

// NewType creates a Type from a reflect.Value.
func NewType(v reflect.Value) (*Type, error) {
	t := v.Type()
	if _, exists := cacheTypeMap[toTypeID(t)]; exists {
		typ := &Type{Kind: REF, Ref: toTypeID(t)}
		setAttribute(v, typ)
		return typ, nil
	}
	typ, err := newType(v)
	if err != nil {
		return nil, err
	}
	setAttribute(v, typ)
	mtdNames := make([]string, 0, t.NumMethod())
	for i := range t.NumMethod() {
		mtdNames = append(mtdNames, t.Method(i).Name)
	}
	typ.MethodNames = mtdNames
	return typ, nil
}

func setAttribute(v reflect.Value, typ *Type) {
	t := v.Type()
	if t.Implements(stepType) {
		typ.Step = true
	}
	if t.Kind() == reflect.Interface && v.IsValid() && v.Elem().IsValid() {
		if IsStepFuncType(v.Elem().Type()) {
			typ.StepFunc = true
		}
	} else if IsStepFuncType(t) {
		typ.StepFunc = true
	}
	if t.Implements(leftArrowFuncType) {
		typ.LeftArrowFunc = true
	}
}

//nolint:cyclop
func newType(v reflect.Value) (*Type, error) {
	if !v.IsValid() {
		return &Type{Kind: INVALID}, nil
	}
	t := v.Type()
	switch t {
	case errorType:
		return &Type{Kind: ERROR}, nil
	case ctxType:
		return &Type{Kind: CONTEXT}, nil
	case schemaStepType:
		return &Type{Kind: SCHEMASTEP}, nil
	}
	switch t.Kind() {
	case reflect.Invalid:
		return &Type{Kind: INVALID}, nil
	case reflect.Int:
		return &Type{Kind: INT}, nil
	case reflect.Int8:
		return &Type{Kind: INT8}, nil
	case reflect.Int16:
		return &Type{Kind: INT16}, nil
	case reflect.Int32:
		return &Type{Kind: INT32}, nil
	case reflect.Int64:
		return &Type{Kind: INT64}, nil
	case reflect.Uint:
		return &Type{Kind: UINT}, nil
	case reflect.Uint8:
		return &Type{Kind: UINT8}, nil
	case reflect.Uint16:
		return &Type{Kind: UINT16}, nil
	case reflect.Uint32:
		return &Type{Kind: UINT32}, nil
	case reflect.Uint64:
		return &Type{Kind: UINT64}, nil
	case reflect.Uintptr:
		return &Type{Kind: UINTPTR}, nil
	case reflect.Float32:
		return &Type{Kind: FLOAT32}, nil
	case reflect.Float64:
		return &Type{Kind: FLOAT64}, nil
	case reflect.Bool:
		return &Type{Kind: BOOL}, nil
	case reflect.String:
		return &Type{Kind: STRING}, nil
	case reflect.Slice:
		return newSliceType(v)
	case reflect.Array:
		return newArrayType(v)
	case reflect.Map:
		return newMapType(v)
	case reflect.Struct:
		return newStructType(v)
	case reflect.Func:
		return newFuncType(v)
	case reflect.Pointer:
		return newPointerType(v)
	case reflect.Interface:
		return newAnyType(v)
	}
	return nil, fmt.Errorf("unsupported wasm plugin type: %s", t)
}

// NewPointerType creates a pointer Type from a reflect.Value.
func newPointerType(v reflect.Value) (*Type, error) {
	elem, err := NewType(newZeroValue(v.Type().Elem()))
	if err != nil {
		return nil, err
	}
	return &Type{
		Kind: POINTER,
		Pointer: &PointerType{
			Elem: elem,
		},
	}, nil
}

func (t *PointerType) ToReflect() (reflect.Type, error) {
	elem, err := t.Elem.ToReflect()
	if err != nil {
		return nil, err
	}
	return reflect.New(elem).Type(), nil
}

func (t *PointerType) String() string {
	return "*" + t.Elem.String()
}

// newAnyType creates a any Type from a reflect.Value.
func newAnyType(v reflect.Value) (*Type, error) {
	var elem *Type
	if v.Elem().IsValid() {
		e, err := NewType(v.Elem())
		if err != nil {
			return nil, err
		}
		elem = e
	} else {
		elem = &Type{Kind: INVALID}
	}
	return &Type{
		Kind: ANY,
		Any: &AnyType{
			Elem: elem,
		},
	}, nil
}

func (t *AnyType) ToReflect() (reflect.Type, error) {
	return reflect.TypeOf((*any)(nil)).Elem(), nil
}

func (t *AnyType) String() string {
	return fmt.Sprintf("any(%s)", t.Elem.String())
}

// newSliceType creates a slice Type from a reflect.Value.
func newSliceType(v reflect.Value) (*Type, error) {
	t := v.Type()
	if t.Elem().Kind() == reflect.Uint8 {
		return &Type{Kind: BYTES}, nil
	}
	ret := &Type{
		Kind:  SLICE,
		Slice: &SliceType{},
	}
	cacheTypeMap[toTypeID(t)] = ret
	elem, err := NewType(newZeroValue(t.Elem()))
	if err != nil {
		return nil, err
	}
	ret.Slice.Elem = elem
	return ret, nil
}

func (t *SliceType) ToReflect() (reflect.Type, error) {
	elem, err := t.Elem.ToReflect()
	if err != nil {
		return nil, err
	}
	return reflect.SliceOf(elem), nil
}

func (t *SliceType) String() string {
	return "[]" + t.Elem.String()
}

// newArrayType creates a array Type from a reflect.Value.
func newArrayType(v reflect.Value) (*Type, error) {
	t := v.Type()
	ret := &Type{
		Kind: ARRAY,
		Array: &ArrayType{
			Num: t.Len(),
		},
	}
	cacheTypeMap[toTypeID(t)] = ret
	elem, err := NewType(newZeroValue(t.Elem()))
	if err != nil {
		return nil, err
	}
	ret.Array.Elem = elem
	return ret, nil
}

func (t *ArrayType) ToReflect() (reflect.Type, error) {
	elem, err := t.Elem.ToReflect()
	if err != nil {
		return nil, err
	}
	return reflect.ArrayOf(t.Num, elem), nil
}

func (t *ArrayType) String() string {
	return fmt.Sprintf("[%d]%s", t.Num, t.Elem.String())
}

// newMapType creates a map Type from a reflect.Value.
func newMapType(v reflect.Value) (*Type, error) {
	t := v.Type()
	ret := &Type{
		Kind: MAP,
		Map:  &MapType{},
	}
	cacheTypeMap[toTypeID(t)] = ret
	key, err := NewType(newZeroValue(t.Key()))
	if err != nil {
		return nil, err
	}
	value, err := NewType(newZeroValue(t.Elem()))
	if err != nil {
		return nil, err
	}
	ret.Map.Key = key
	ret.Map.Value = value
	return ret, nil
}

func (t *MapType) ToReflect() (reflect.Type, error) {
	k, err := t.Key.ToReflect()
	if err != nil {
		return nil, err
	}
	v, err := t.Value.ToReflect()
	if err != nil {
		return nil, err
	}
	return reflect.MapOf(k, v), nil
}

func (t *MapType) String() string {
	return fmt.Sprintf("map[%s]%s", t.Key.String(), t.Value.String())
}

// newStructType creates a struct Type from a reflect.Value.
// Only creates top-level struct information without field type details.
func newStructType(v reflect.Value) (*Type, error) {
	t := v.Type()
	ret := &Type{
		Kind: STRUCT,
		Struct: &StructType{
			Fields: make([]*NameWithType, 0, t.NumField()),
		},
	}
	cacheTypeMap[toTypeID(t)] = ret
	for i := range t.NumField() {
		field := t.Field(i)
		// Only store field names, not their detailed types
		ret.Struct.Fields = append(ret.Struct.Fields, &NameWithType{
			Name: field.Name,
			Type: &Type{Kind: INVALID}, // Placeholder, actual type will be resolved on guest side
		})
	}
	return ret, nil
}

func (t *StructType) ToReflect() (reflect.Type, error) {
	fields := make([]reflect.StructField, 0, len(t.Fields))
	for _, field := range t.Fields {
		if len(field.Name) == 0 {
			continue
		}
		if unicode.IsLower(rune(field.Name[0])) {
			continue
		}
		// Skip INVALID field types since they are placeholders
		if field.Type.Kind == INVALID {
			continue
		}
		typ, err := field.Type.ToReflect()
		if err != nil {
			return nil, err
		}
		fields = append(fields, reflect.StructField{
			Name: field.Name,
			Type: typ,
		})
	}
	return reflect.StructOf(fields), nil
}

func (t *StructType) String() string {
	fields := make([]string, 0, len(t.Fields))
	for _, field := range t.Fields {
		// Skip INVALID field types since they are placeholders
		if field.Type.Kind == INVALID {
			continue
		}
		fields = append(fields, field.String())
	}
	return fmt.Sprintf("struct{%s}", strings.Join(fields, " "))
}

// newFuncType creates a function Type from a reflect.Value.
func newFuncType(v reflect.Value) (*Type, error) {
	t := v.Type()
	ret := &Type{Kind: FUNC, Func: &FuncType{}}
	cacheTypeMap[toTypeID(t)] = ret
	for i := range t.NumIn() {
		typ, err := NewType(newZeroValue(t.In(i)))
		if err != nil {
			return nil, err
		}
		ret.Func.Args = append(ret.Func.Args, typ)
	}
	for i := range t.NumOut() {
		typ, err := NewType(newZeroValue(t.Out(i)))
		if err != nil {
			return nil, err
		}
		ret.Func.Return = append(ret.Func.Return, typ)
	}
	return ret, nil
}

func (t *FuncType) ToReflect() (reflect.Type, error) {
	args := make([]reflect.Type, 0, len(t.Args))
	for _, arg := range t.Args {
		typ, err := arg.ToReflect()
		if err != nil {
			return nil, err
		}
		args = append(args, typ)
	}
	ret := make([]reflect.Type, 0, len(t.Return))
	for _, r := range t.Return {
		typ, err := r.ToReflect()
		if err != nil {
			return nil, err
		}
		ret = append(ret, typ)
	}
	return reflect.FuncOf(args, ret, false), nil
}

func (t *FuncType) String() string {
	args := make([]string, 0, len(t.Args))
	for _, arg := range t.Args {
		args = append(args, arg.String())
	}
	rets := make([]string, 0, len(t.Return))
	for _, ret := range t.Return {
		rets = append(rets, ret.String())
	}
	return fmt.Sprintf(
		"func(%s)%s",
		strings.Join(args, ", "),
		strings.Join(rets, ", "),
	)
}

func (t *NameWithType) String() string {
	return fmt.Sprintf("%s %s", t.Name, t.Type.String())
}

//nolint:cyclop
func (t *Type) ToReflect() (reflect.Type, error) {
	switch t.Kind {
	case INT:
		return reflect.TypeOf(int(0)), nil
	case INT8:
		return reflect.TypeOf(int8(0)), nil
	case INT16:
		return reflect.TypeOf(int16(0)), nil
	case INT32:
		return reflect.TypeOf(int32(0)), nil
	case INT64:
		return reflect.TypeOf(int64(0)), nil
	case UINT:
		return reflect.TypeOf(uint(0)), nil
	case UINT8:
		return reflect.TypeOf(uint8(0)), nil
	case UINT16:
		return reflect.TypeOf(uint16(0)), nil
	case UINT32:
		return reflect.TypeOf(uint32(0)), nil
	case UINT64:
		return reflect.TypeOf(uint64(0)), nil
	case UINTPTR:
		return reflect.TypeOf(uintptr(0)), nil
	case FLOAT32:
		return reflect.TypeOf(float32(0)), nil
	case FLOAT64:
		return reflect.TypeOf(float64(0)), nil
	case BOOL:
		return reflect.TypeOf(false), nil
	case STRING:
		return reflect.TypeOf(""), nil
	case BYTES:
		return reflect.TypeOf([]byte{}), nil
	case FUNC:
		return t.Func.ToReflect()
	case MAP:
		return t.Map.ToReflect()
	case SLICE:
		return t.Slice.ToReflect()
	case ARRAY:
		return t.Array.ToReflect()
	case STRUCT:
		return t.Struct.ToReflect()
	case POINTER:
		return t.Pointer.ToReflect()
	case ANY:
		return t.Any.ToReflect()
	case ERROR:
		return errorType, nil
	case CONTEXT:
		return ctxType, nil
	case SCHEMASTEP:
		return schemaStepType, nil
	}
	return nil, fmt.Errorf("failed to get reflect.Type from %s", t)
}

func (t *Type) FieldTypeByName(name string) *Type {
	if t.Kind == POINTER {
		return t.Pointer.Elem.FieldTypeByName(name)
	}
	for _, field := range t.Struct.Fields {
		if field.Name == name {
			return field.Type
		}
	}
	return nil
}

func (t *Type) HasField(name string) bool {
	if t.Kind == POINTER {
		return t.Pointer.Elem.HasField(name)
	}
	if t.Kind == ANY {
		return t.Any.Elem.HasField(name)
	}
	if t.Kind != STRUCT {
		return false
	}
	return t.Struct.HasField(name)
}

func (t *Type) HasMethod(name string) bool {
	for _, mtdName := range t.MethodNames {
		if mtdName == name {
			return true
		}
	}
	if t.Kind == POINTER {
		return t.Pointer.Elem.HasMethod(name)
	}
	return false
}

func (t *Type) IsStruct() bool {
	if t.Kind == POINTER {
		return t.Pointer.Elem.IsStruct()
	}
	return t.Kind == STRUCT
}

func (t *Type) String() string {
	switch t.Kind {
	case INVALID:
		return nullString
	case INT, INT8, INT16, INT32, INT64, UINT, UINT8, UINT16, UINT32, UINT64, UINTPTR,
		FLOAT32, FLOAT64, BYTES, STRING, BOOL, ANY:
		return string(t.Kind)
	case SLICE:
		return t.Slice.String()
	case ARRAY:
		return t.Array.String()
	case MAP:
		return t.Map.String()
	case STRUCT:
		return t.Struct.String()
	case FUNC:
		return t.Func.String()
	case POINTER:
		return t.Pointer.String()
	}
	return string(t.Kind)
}

func TypeRefMap() map[string]*Type {
	return cacheTypeMap
}

func ResolveRef(t *Type, typeRefMap map[string]*Type) (*Type, error) {
	return resolveRef(t, typeRefMap, make(map[*Type]*Type))
}

func resolveRef(t *Type, typeRefMap map[string]*Type, resolvedMap map[*Type]*Type) (*Type, error) {
	if resolved, exists := resolvedMap[t]; exists {
		return resolved, nil
	}
	ret := &Type{
		MethodNames:   t.MethodNames,
		Step:          t.Step,
		StepFunc:      t.StepFunc,
		LeftArrowFunc: t.LeftArrowFunc,
	}
	switch t.Kind {
	case INVALID, INT, INT8, INT16, INT32, INT64,
		UINT, UINT8, UINT16, UINT32, UINT64, UINTPTR,
		FLOAT32, FLOAT64, BOOL, STRING, BYTES, ANY, ERROR, CONTEXT, SCHEMASTEP:
		return t, nil
	case FUNC:
		ret.Kind = FUNC
		resolvedMap[t] = ret
		args := make([]*Type, 0, len(t.Func.Args))
		for _, arg := range t.Func.Args {
			typ, err := resolveRef(arg, typeRefMap, resolvedMap)
			if err != nil {
				return nil, err
			}
			args = append(args, typ)
		}
		rets := make([]*Type, 0, len(t.Func.Return))
		for _, ret := range t.Func.Return {
			typ, err := resolveRef(ret, typeRefMap, resolvedMap)
			if err != nil {
				return nil, err
			}
			rets = append(rets, typ)
		}
		ret.Func = &FuncType{Args: args, Return: rets}
		return ret, nil
	case MAP:
		ret.Kind = MAP
		resolvedMap[t] = ret
		key, err := resolveRef(t.Map.Key, typeRefMap, resolvedMap)
		if err != nil {
			return nil, err
		}
		value, err := resolveRef(t.Map.Value, typeRefMap, resolvedMap)
		if err != nil {
			return nil, err
		}
		ret.Map = &MapType{Key: key, Value: value}
		return ret, nil
	case SLICE:
		ret.Kind = SLICE
		resolvedMap[t] = ret
		elem, err := resolveRef(t.Slice.Elem, typeRefMap, resolvedMap)
		if err != nil {
			return nil, err
		}
		ret.Slice = &SliceType{Elem: elem}
		return ret, nil
	case ARRAY:
		ret.Kind = ARRAY
		resolvedMap[t] = ret
		elem, err := resolveRef(t.Array.Elem, typeRefMap, resolvedMap)
		if err != nil {
			return nil, err
		}
		ret.Array = &ArrayType{Num: t.Array.Num, Elem: elem}
		return ret, nil
	case STRUCT:
		ret.Kind = STRUCT
		resolvedMap[t] = ret
		fields := make([]*NameWithType, 0, len(t.Struct.Fields))
		for _, field := range t.Struct.Fields {
			typ, err := resolveRef(field.Type, typeRefMap, resolvedMap)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &NameWithType{
				Name: field.Name,
				Type: typ,
			})
		}
		ret.Struct = &StructType{Fields: fields}
		return ret, nil
	case POINTER:
		ret.Kind = POINTER
		resolvedMap[t] = ret
		elem, err := resolveRef(t.Pointer.Elem, typeRefMap, resolvedMap)
		if err != nil {
			return nil, err
		}
		ret.Pointer = &PointerType{Elem: elem}
		return ret, nil
	case REF:
		refType, exists := typeRefMap[t.Ref]
		if !exists {
			return nil, fmt.Errorf("failed to find type from %s type-id", t.Ref)
		}
		return resolveRef(refType, typeRefMap, resolvedMap)
	}
	return nil, fmt.Errorf("unexpected type: %s", t)
}

func newZeroValue(t reflect.Type) reflect.Value {
	return reflect.New(t).Elem()
}

// IsStepFuncType compares with `func(ctx *context.Context, step *schema.Step) *context.Context` type.
func IsStepFuncType(t reflect.Type) bool {
	if t.Kind() != reflect.Func {
		return false
	}
	if t.NumIn() != 2 {
		return false
	}
	if t.NumOut() != 1 {
		return false
	}
	if t.In(0) != ctxType {
		return false
	}
	if t.In(1) != schemaStepType {
		return false
	}
	if t.Out(0) != ctxType {
		return false
	}
	return true
}
