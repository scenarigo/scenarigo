package wasm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

var cacheTypeMap = map[reflect.Type]*Type{}

// NameWithType represents a named type from a WASM plugin.
type NameWithType struct {
	Name string `json:"name"`
	Type *Type  `json:"type"`
}

// Type represents a type information from a WASM plugin.
type Type struct {
	Kind    Kind         `json:"kind"`
	Methods []*Method    `json:"methods"`
	Any     *AnyType     `json:"any"`
	Pointer *PointerType `json:"pointer"`
	Struct  *StructType  `json:"struct"`
	Slice   *SliceType   `json:"slice"`
	Array   *ArrayType   `json:"array"`
	Map     *MapType     `json:"map"`
	Func    *FuncType    `json:"func"`
}

type Kind string

const (
	INVALID Kind = "invalid"
	INT     Kind = "int"
	INT8    Kind = "int8"
	INT16   Kind = "int16"
	INT32   Kind = "int32"
	INT64   Kind = "int64"
	UINT    Kind = "uint"
	UINT8   Kind = "uint8"
	UINT16  Kind = "uint16"
	UINT32  Kind = "uint32"
	UINT64  Kind = "uint64"
	UINTPTR Kind = "uintptr"
	FLOAT32 Kind = "float32"
	FLOAT64 Kind = "float64"
	STRING  Kind = "string"
	BYTES   Kind = "bytes"
	BOOL    Kind = "bool"
	STRUCT  Kind = "struct"
	SLICE   Kind = "slice"
	ARRAY   Kind = "array"
	MAP     Kind = "map"
	FUNC    Kind = "func"
	POINTER Kind = "pointer"
	ANY     Kind = "any"
)

// PointerTyep represents pointer type information from a WASM plugin.
type PointerType struct {
	Elem *Type `json:"elem"`
}

// AnyType represents any type information from a WASM plugin.
type AnyType struct {
	Elem *Type `json:"elem"`
}

type Method struct {
	Name string    `json:"name"`
	Type *FuncType `json:"type"`
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

// NewType creates a Type from a reflect.Type.
func NewType(t reflect.Type) (*Type, error) {
	if typ, exists := cacheTypeMap[t]; exists {
		return typ, nil
	}
	typ, err := newType(t)
	if err != nil {
		return nil, err
	}
	mtds, err := newMethods(t)
	if err != nil {
		return nil, err
	}
	_ = mtds
	//typ.Methods = mtds
	return typ, nil
}

func newType(t reflect.Type) (*Type, error) {
	if t == nil {
		return &Type{Kind: INVALID}, nil
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
		return NewSliceType(t)
	case reflect.Array:
		return NewArrayType(t)
	case reflect.Map:
		return NewMapType(t)
	case reflect.Struct:
		return NewStructType(t)
	case reflect.Func:
		return NewFuncType(t)
	case reflect.Pointer:
		return NewPointerType(t)
	case reflect.Interface:
		return NewAnyType(t)
	}
	return nil, fmt.Errorf("unsupported wasm plugin type: %s", t)
}

func newMethods(t reflect.Type) ([]*Method, error) {
	ret := make([]*Method, 0, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		mtd := t.Method(i)
		typ, err := NewType(mtd.Type)
		if err != nil {
			continue
		}
		ret = append(ret, &Method{
			Name: mtd.Name,
			Type: typ.Func,
		})
	}
	return ret, nil
}

// NewPointerType creates a pointer Type from a reflect.Value.
func NewPointerType(t reflect.Type) (*Type, error) {
	elem, err := NewType(t.Elem())
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

// NewAnyType creates a any Type from a reflect.Value.
func NewAnyType(t reflect.Type) (*Type, error) {
	return &Type{
		Kind: ANY,
		Any:  &AnyType{},
	}, nil
}

func (t *AnyType) ToReflect() (reflect.Type, error) {
	return reflect.TypeOf((*any)(nil)).Elem(), nil
}

func (t *AnyType) String() string {
	return "any"
}

// NewSliceType creates a slice Type from a reflect.Value.
func NewSliceType(t reflect.Type) (*Type, error) {
	if t.Elem().Kind() == reflect.Uint8 {
		return &Type{Kind: BYTES}, nil
	}
	ret := &Type{
		Kind:  SLICE,
		Slice: &SliceType{},
	}
	cacheTypeMap[t] = ret
	elem, err := NewType(t.Elem())
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

// NewArrayType creates a array Type from a reflect.Value.
func NewArrayType(t reflect.Type) (*Type, error) {
	ret := &Type{
		Kind: ARRAY,
		Array: &ArrayType{
			Num: t.Len(),
		},
	}
	cacheTypeMap[t] = ret
	elem, err := NewType(t.Elem())
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

// NewMapType creates a map Type from a reflect.Value.
func NewMapType(t reflect.Type) (*Type, error) {
	ret := &Type{
		Kind: MAP,
		Map:  &MapType{},
	}
	cacheTypeMap[t] = ret
	key, err := NewType(t.Key())
	if err != nil {
		return nil, err
	}
	value, err := NewType(t.Elem())
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

// NewStructType creates a struct Type from a reflect.Value.
func NewStructType(t reflect.Type) (*Type, error) {
	ret := &Type{
		Kind: STRUCT,
		Struct: &StructType{
			Fields: make([]*NameWithType, 0, t.NumField()),
		},
	}
	cacheTypeMap[t] = ret
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		typ, err := NewType(field.Type)
		if err != nil {
			return nil, err
		}
		ret.Struct.Fields = append(ret.Struct.Fields, &NameWithType{
			Name: field.Name,
			Type: typ,
		})
	}
	return ret, nil
}

func (t *StructType) ToReflect() (reflect.Type, error) {
	fields := make([]reflect.StructField, 0, len(t.Fields))
	for _, field := range t.Fields {
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
		fields = append(fields, field.String())
	}
	return fmt.Sprintf("struct{%s}", strings.Join(fields, " "))
}

// NewFuncType creates a function Type from a reflect.Value.
func NewFuncType(t reflect.Type) (*Type, error) {
	ret := &Type{Kind: FUNC, Func: &FuncType{}}
	cacheTypeMap[t] = ret
	for i := range t.NumIn() {
		typ, err := NewType(t.In(i))
		if err != nil {
			return nil, err
		}
		ret.Func.Args = append(ret.Func.Args, typ)
	}
	for i := range t.NumOut() {
		typ, err := NewType(t.Out(i))
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
	for _, mtd := range t.Methods {
		if mtd.Name == name {
			return true
		}
	}
	return false
}

func (t *Type) MethodTypeByName(name string) *FuncType {
	for _, mtd := range t.Methods {
		if mtd.Name == name {
			return mtd.Type
		}
	}
	return nil
}

func (t *Type) IsStruct() bool {
	if t.Kind == POINTER {
		return t.Pointer.Elem.IsStruct()
	}
	if t.Kind == ANY {
		return t.Any.Elem.IsStruct()
	}
	return t.Kind == STRUCT
}

func (t *Type) String() string {
	switch t.Kind {
	case INVALID:
		return "null"
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
