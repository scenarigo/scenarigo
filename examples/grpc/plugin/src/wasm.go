package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"unsafe"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
	"github.com/scenarigo/scenarigo/plugin"
)

//go:wasmexport __alloc
func __alloc(size int32) int32 {
	b := make([]byte, size)
	return int32(uintptr(unsafe.Pointer(&b[0])))
}

//go:wasmexport __scenarigo_plugin_init
func __scenarigo_plugin_init(retAddr int32, retLen int32) {
	returnValue(plugin.ExportedValue{
		GRPCClients: []string{
			"PingClient",
			"EchoClient",
		},
	}, retAddr, retLen)
}

//go:wasmexport __scenarigo_plugin_setup
func __scenarigo_plugin_setup(retAddr, retLen int32) {
	fmt.Println("run setup")
	plugin.RunSetup()
}

//go:wasmexport __scenarigo_plugin_teardown
func __scenarigo_plugin_teardown(retAddr, retLen int32) {
	fmt.Println("run teardown")
	plugin.RunTeardown()
}

//go:wasmexport __scenarigo_plugin_grpc_client_exists_method
func __scenarigo_plugin_grpc_client_exists_method(reqAddr, reqLen, retAddr, retLen int32) {
	reqStr := unsafe.String((*byte)(unsafe.Pointer(uintptr(uint64(reqAddr)))), reqLen)
	var req plugin.ExistsMethodRequest
	if err := json.Unmarshal([]byte(reqStr), &req); err != nil {
		panic(err)
	}
	var client reflect.Value
	switch req.ClientName {
	case "EchoClient":
		client = reflect.ValueOf(EchoClient)
	case "PingClient":
		client = reflect.ValueOf(PingClient)
	default:
		panic(fmt.Sprintf("unknown clientName: %q", req.ClientName))
	}
	exists, err := existsMethod(client, req.ClientName, req.MethodName)
	res := &plugin.ExistsMethodResponse{Exists: exists}
	if err != nil {
		res.Error = err.Error()
	}
	returnValue(res, retAddr, retLen)
}

func returnValue(v any, retAddr, retLen int32) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	*(*int32)(unsafe.Pointer(uintptr(retAddr))) = int32(uintptr(unsafe.Pointer(&b[0])))
	*(*int32)(unsafe.Pointer(uintptr(retLen))) = int32(len(b))
}

func existsMethod(v reflect.Value, clientName, methodName string) (bool, error) {
	var method reflect.Value
	for {
		if !v.IsValid() {
			return false, fmt.Errorf("client %q is invalid", clientName)
		}
		method = v.MethodByName(methodName)
		if method.IsValid() {
			// method found
			break
		}
		switch v.Kind() {
		case reflect.Interface, reflect.Ptr:
			v = v.Elem()
		default:
			return false, fmt.Errorf(`method "%s.%s" not found`, clientName, methodName)
		}
	}
	if err := validateMethod(method); err != nil {
		return false, fmt.Errorf(`"%s.%s" must be "func(context.Context, proto.Message, ...grpc.CallOption) (proto.Message, error): %s"`, clientName, methodName, err)
	}
	return true, nil
}

var (
	typeContext  = reflect.TypeOf((*context.Context)(nil)).Elem()
	typeMessage  = reflect.TypeOf((*proto.Message)(nil)).Elem()
	typeCallOpts = reflect.TypeOf([]grpc.CallOption(nil))
)

func validateMethod(method reflect.Value) error {
	if !method.IsValid() {
		return errors.New("invalid")
	}
	if method.Kind() != reflect.Func {
		return errors.New("not function")
	}
	if method.IsNil() {
		return errors.New("method is nil")
	}

	mt := method.Type()
	if n := mt.NumIn(); n != 3 {
		return fmt.Errorf("number of arguments must be 3 but got %d", n)
	}
	if t := mt.In(0); !t.Implements(typeContext) {
		return fmt.Errorf("first argument must be context.Context but got %s", t.String())
	}
	if t := mt.In(1); !t.Implements(typeMessage) {
		return fmt.Errorf("second argument must be proto.Message but got %s", t.String())
	}
	if t := mt.In(2); t != typeCallOpts {
		return fmt.Errorf("third argument must be []grpc.CallOption but got %s", t.String())
	}
	if n := mt.NumOut(); n != 2 {
		return fmt.Errorf("number of return values must be 2 but got %d", n)
	}
	if t := mt.Out(0); !t.Implements(typeMessage) {
		return fmt.Errorf("first return value must be proto.Message but got %s", t.String())
	}
	if t := mt.Out(1); !t.Implements(reflectutil.TypeError) {
		return fmt.Errorf("second return value must be error but got %s", t.String())
	}

	return nil
}

func main() {}
