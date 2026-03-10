package val

import (
	"reflect"
	"sync"
	"time"
)

var (
	tm      sync.RWMutex
	types   []Type
	typeMap map[string]Type
)

func init() {
	types = []Type{}
	typeMap = map[string]Type{}

	// undefined types
	RegisterType(anyType)

	// primitive types
	RegisterType(intType)
	RegisterType(uintType)
	RegisterType(floatType)
	RegisterType(boolType)
	RegisterType(stringType)

	RegisterType(bytesType)
	RegisterType(timeType)
	RegisterType(durationType)
	RegisterType(nilType)
}

// RegisterType registers an abstract type t.
// Later registered types take precedence over earlier ones.
func RegisterType(t Type) {
	tm.Lock()
	defer tm.Unlock()
	types = append(types, t)
	typeMap[t.Name()] = t
}

// GetType gets registered type by name.
// This function returns nil if a type is not found by the given name.
func GetType(name string) Type {
	tm.RLock()
	defer tm.RUnlock()
	if t, ok := typeMap[name]; ok {
		return t
	}
	return nil
}

// Type represents an abstract operand type.
type Type interface {
	Name() string
	NewValue(any) (Value, error)
	Convert(Value) (Value, error)
}

var (
	typeInt64   = reflect.TypeFor[int64]()
	typeUint64  = reflect.TypeFor[uint64]()
	typeFloat64 = reflect.TypeFor[float64]()
	typeBool    = reflect.TypeFor[bool]()
	typeString  = reflect.TypeFor[string]()
	typeBytes   = reflect.TypeFor[[]byte]()
	typeTime    = reflect.TypeFor[time.Time]()
)

type basicType struct {
	name     string
	newValue func(v any) (Value, error)
	convert  func(v Value) (Value, error)
}

// Name implements Type interface.
func (t basicType) Name() string {
	return t.name
}

// NewValue implements Type interface.
func (t basicType) NewValue(v any) (Value, error) {
	return t.newValue(v)
}

// Convert implements Type interface.
func (t basicType) Convert(v Value) (Value, error) {
	return t.convert(v)
}
