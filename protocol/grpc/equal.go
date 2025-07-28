package grpc

import (
	"reflect"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

func init() {
	assert.RegisterCustomEqualer(assert.EqualerFunc(equalEnum))
	assert.RegisterCustomEqualer(assert.EqualerFunc(equalMessage))
}

var (
	protoMessage = reflect.TypeOf((*proto.Message)(nil)).Elem()
	uint64Type   = reflect.TypeOf(uint64(0))
)

func equalEnum(expected any, got any) (bool, error) {
	enum, ok := got.(protoreflect.Enum)
	if !ok {
		return false, nil
	}
	number := enum.Descriptor().Values().ByNumber(enum.Number())
	if number == nil {
		// If enum.Number() is a reserved value or unknown value, the number variable will be nil.
		return false, nil
	}
	rexp := reflect.ValueOf(expected)
	if rexp.CanConvert(uint64Type) {
		// specified direct number.
		exp := rexp.Convert(uint64Type).Uint()
		return exp == uint64(enum.Number()), nil
	}
	s, ok := expected.(string)
	if !ok {
		return false, nil
	}
	if string(number.Name()) == s {
		return true, nil
	}
	return false, nil
}

func equalMessage(expected any, got any) (bool, error) {
	// use the pointer to the value if the pointer type implements proto.Message
	e, ok, _ := reflectutil.ConvertInterface(protoMessage, expected)
	if ok {
		expected = e
	}
	g, ok, _ := reflectutil.ConvertInterface(protoMessage, got)
	if ok {
		got = g
	}
	em, ok := expected.(proto.Message)
	if !ok {
		return false, nil
	}
	gm, ok := got.(proto.Message)
	if !ok {
		return false, nil
	}
	if proto.Equal(em, gm) {
		return true, nil
	}
	return false, nil
}
