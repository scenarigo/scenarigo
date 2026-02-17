package wasm

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/schema"
)

func TestEncodeValue_Context(t *testing.T) {
	// Test CONTEXT case in EncodeValue
	ctx := context.FromT(t)
	ctxValue := reflect.ValueOf(ctx)

	result, err := EncodeValue(ctxValue)
	if err != nil {
		t.Fatalf("EncodeValue() context error = %v", err)
	}

	if result.Type == nil {
		t.Fatal("EncodeValue() Type should not be nil")
	}

	if result.Type.Kind != CONTEXT {
		t.Errorf("EncodeValue() context Type.Kind = %v, want %v", result.Type.Kind, CONTEXT)
	}
}

func TestEncodeValue_Error(t *testing.T) {
	// Test ERROR case in EncodeValue
	// Create a value that has error interface type
	testErr := errors.New("test error message")

	// Create a reflect.Value with interface type error, not concrete type
	errorInterfaceType := reflect.TypeFor[error]()
	errValue := reflect.New(errorInterfaceType).Elem()
	errValue.Set(reflect.ValueOf(testErr))

	result, err := EncodeValue(errValue)
	if err != nil {
		t.Fatalf("EncodeValue() error = %v", err)
	}

	if result.Type == nil {
		t.Fatal("EncodeValue() Type should not be nil")
	}

	if result.Type.Kind != ERROR {
		t.Errorf("EncodeValue() error Type.Kind = %v, want %v", result.Type.Kind, ERROR)
	}

	if result.Value != "test error message" {
		t.Errorf("EncodeValue() error Value = %v, want %v", result.Value, "test error message")
	}
}

func TestEncodeValue_NewTypeError(t *testing.T) {
	// Test error case when NewType fails
	// Create a value that would cause NewType to fail
	invalidValue := reflect.ValueOf(make(chan int)) // channels are not supported

	_, err := EncodeValue(invalidValue)
	if err == nil {
		t.Error("EncodeValue() should return error for unsupported type")
	}
}

func TestDecodeValueWithType_ContextErrors(t *testing.T) {
	ctxType := reflect.TypeFor[*context.Context]()

	// Test invalid JSON for context (should fail at json.Unmarshal step - lines 53-55)
	_, err := DecodeValueWithType(ctxType, []byte(`{invalid json}`))
	if err == nil {
		t.Error("DecodeValueWithType() should return error for invalid context JSON")
	}
}

func TestDecodeValueWithType_NonContextType(t *testing.T) {
	// Test DecodeValueWithType with non-context type (lines 62-67)
	stringType := reflect.TypeFor[string]()

	// Test successful decode
	result, err := DecodeValueWithType(stringType, []byte(`"hello world"`))
	if err != nil {
		t.Fatalf("DecodeValueWithType() non-context type error = %v", err)
	}

	if !result.IsValid() {
		t.Error("DecodeValueWithType() should return valid value")
	}

	if result.Interface() != "hello world" {
		t.Errorf("DecodeValueWithType() = %v, want hello world", result.Interface())
	}

	// Test decode error with invalid JSON for target type (line 65)
	_, err = DecodeValueWithType(stringType, []byte(`123`))
	if err == nil {
		t.Error("DecodeValueWithType() should return error for type mismatch")
	}
}

func TestDecodeValueWithType_ContextSuccess(t *testing.T) {
	ctxType := reflect.TypeFor[*context.Context]()

	// Create a serializable context
	originalCtx := context.FromT(t)
	sctx := originalCtx.ToSerializable()

	// Marshal to JSON
	data, err := json.Marshal(sctx)
	if err != nil {
		t.Fatalf("Failed to marshal serializable context: %v", err)
	}

	// Test successful decoding
	result, err := DecodeValueWithType(ctxType, data)
	if err != nil {
		t.Fatalf("DecodeValueWithType() context error = %v", err)
	}

	if !result.IsValid() {
		t.Error("DecodeValueWithType() should return valid value")
	}

	if result.Type() != ctxType {
		t.Errorf("DecodeValueWithType() result type = %v, want %v", result.Type(), ctxType)
	}
}

func TestValue_Struct(t *testing.T) {
	// Test the Value struct itself
	v := &Value{
		ID:    "test-id",
		Value: "test-value",
		Type: &Type{
			Kind: STRING,
		},
	}

	if v.ID != "test-id" {
		t.Errorf("Value.ID = %v, want test-id", v.ID)
	}

	if v.Value != "test-value" {
		t.Errorf("Value.Value = %v, want test-value", v.Value)
	}

	if v.Type.Kind != STRING {
		t.Errorf("Value.Type.Kind = %v, want %v", v.Type.Kind, STRING)
	}
}

func TestEncodeValue_StepFuncBypassJSON(t *testing.T) {
	// Test that StepFunc types bypass JSON marshaling (lines 39-41 in value.go)
	stepFunc := func(ctx *context.Context, step *schema.Step) *context.Context {
		return ctx
	}
	value := reflect.ValueOf(stepFunc)

	result, err := EncodeValue(value)
	if err != nil {
		t.Fatalf("EncodeValue() stepfunc error = %v", err)
	}

	// StepFunc types should have empty Value (bypass JSON marshaling)
	if result.Value != "" {
		t.Errorf("EncodeValue() StepFunc Value = %v, want empty string", result.Value)
	}

	if result.Type == nil {
		t.Fatal("EncodeValue() Type should not be nil")
	}

	if !result.Type.StepFunc {
		t.Error("EncodeValue() should detect StepFunc type")
	}
}

func TestEncodeValue_IDGeneration(t *testing.T) {
	// Test that ID is generated correctly using pointer address
	value := reflect.ValueOf("test")

	result1, err := EncodeValue(value)
	if err != nil {
		t.Fatalf("EncodeValue() error = %v", err)
	}

	result2, err := EncodeValue(value)
	if err != nil {
		t.Fatalf("EncodeValue() error = %v", err)
	}

	// IDs should be different as they're based on the address of reflect.Value
	if result1.ID == result2.ID {
		t.Error("EncodeValue() should generate different IDs for different calls")
	}

	// Both should have non-empty IDs
	if result1.ID == "" {
		t.Error("EncodeValue() ID should not be empty")
	}

	if result2.ID == "" {
		t.Error("EncodeValue() ID should not be empty")
	}
}
