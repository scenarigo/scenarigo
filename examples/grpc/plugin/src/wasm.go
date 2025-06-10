package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"unsafe"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

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
func __scenarigo_plugin_setup(reqAddr, reqLen, retAddr, retLen int32) {
	reqStr := unsafe.String((*byte)(unsafe.Pointer(uintptr(uint64(reqAddr)))), reqLen)
	plugin.RunSetup(reqStr)
}

//go:wasmexport __scenarigo_plugin_teardown
func __scenarigo_plugin_teardown(reqAddr, reqLen, retAddr, retLen int32) {
	reqStr := unsafe.String((*byte)(unsafe.Pointer(uintptr(uint64(reqAddr)))), reqLen)
	plugin.RunTeardown(reqStr)
}

//go:wasmexport __scenarigo_plugin_grpc_client_exists_method
func __scenarigo_plugin_grpc_client_exists_method(reqAddr, reqLen, retAddr, retLen int32) {
	reqStr := unsafe.String((*byte)(unsafe.Pointer(uintptr(uint64(reqAddr)))), reqLen)
	var req plugin.ExistsMethodRequest
	if err := json.Unmarshal([]byte(reqStr), &req); err != nil {
		panic(err)
	}
	res := &plugin.ExistsMethodResponse{}
	method, err := getMethod(req.ClientName, req.MethodName)
	if err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	if err := validateMethod(method); err != nil {
		res.Error = fmt.Sprintf(`"%s.%s" must be "func(context.Context, proto.Message, ...grpc.CallOption) (proto.Message, error): %s"`, req.ClientName, req.MethodName, err)
		returnValue(res, retAddr, retLen)
		return
	}
	res.Exists = true
	returnValue(res, retAddr, retLen)
}

//go:wasmexport __scenarigo_plugin_grpc_client_build_request
func __scenarigo_plugin_grpc_client_build_request(reqAddr, reqLen, retAddr, retLen int32) {
	res := &plugin.BuildResponse{}

	reqStr := unsafe.String((*byte)(unsafe.Pointer(uintptr(uint64(reqAddr)))), reqLen)
	var req plugin.BuildRequest
	if err := json.Unmarshal([]byte(reqStr), &req); err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	method, err := getMethod(req.ClientName, req.MethodName)
	if err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	reqMsg := reflect.New(method.Type().In(1).Elem()).Interface()
	if _, ok := reqMsg.(proto.Message); !ok {
		res.Error = fmt.Sprintf("failed to convert to request message: %T", reqMsg)
		returnValue(res, retAddr, retLen)
		return
	}
	fullName := reqMsg.(proto.Message).ProtoReflect().Descriptor().FullName()
	res.MessageFQDN = string(fullName)
	var fdSet descriptorpb.FileDescriptorSet
	protoregistry.GlobalFiles.RangeFiles(func(fileDescriptor protoreflect.FileDescriptor) bool {
		fdProto := protodesc.ToFileDescriptorProto(fileDescriptor)
		fdSet.File = append(fdSet.File, fdProto)
		return true
	})
	fdSetBytes, err := proto.Marshal(&fdSet)
	if err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	res.FDSet = fdSetBytes
	returnValue(res, retAddr, retLen)
}

//go:wasmexport __scenarigo_plugin_grpc_client_invoke
func __scenarigo_plugin_grpc_client_invoke(reqAddr, reqLen, retAddr, retLen int32) {
	res := &plugin.InvokeResponse{}

	reqStr := unsafe.String((*byte)(unsafe.Pointer(uintptr(uint64(reqAddr)))), reqLen)
	var req plugin.InvokeRequest
	if err := json.Unmarshal([]byte(reqStr), &req); err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	method, err := getMethod(req.ClientName, req.MethodName)
	if err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	reqMsg := reflect.New(method.Type().In(1).Elem()).Interface()
	if _, ok := reqMsg.(proto.Message); !ok {
		res.Error = fmt.Sprintf("failed to convert to request message: %T", reqMsg)
		returnValue(res, retAddr, retLen)
		return
	}
	reqProtoMsg := reqMsg.(proto.Message)
	if err := protojson.Unmarshal(req.Request, reqProtoMsg); err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	resMsg, st, err := invoke(method, reqProtoMsg)
	if err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	fullName := resMsg.ProtoReflect().Descriptor().FullName()
	res.ResponseFQDN = string(fullName)
	resMsgBytes, err := protojson.Marshal(resMsg)
	if err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	res.ResponseBytes = resMsgBytes
	var fdSet descriptorpb.FileDescriptorSet
	protoregistry.GlobalFiles.RangeFiles(func(fileDescriptor protoreflect.FileDescriptor) bool {
		fdProto := protodesc.ToFileDescriptorProto(fileDescriptor)
		fdSet.File = append(fdSet.File, fdProto)
		return true
	})
	fdSetBytes, err := proto.Marshal(&fdSet)
	if err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	res.FDSet = fdSetBytes
	stBytes, err := proto.Marshal(st.Proto())
	if err != nil {
		res.Error = err.Error()
		returnValue(res, retAddr, retLen)
		return
	}
	res.StatusProto = stBytes
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

func getMethod(clientName, methodName string) (reflect.Value, error) {
	var client reflect.Value
	switch clientName {
	case "EchoClient":
		client = reflect.ValueOf(EchoClient)
	case "PingClient":
		client = reflect.ValueOf(PingClient)
	default:
		return reflect.Value{}, fmt.Errorf("unknown clientName: %q", clientName)
	}
	var method reflect.Value
	for {
		if !client.IsValid() {
			return reflect.Value{}, fmt.Errorf("client %q is invalid", clientName)
		}
		method = client.MethodByName(methodName)
		if method.IsValid() {
			// method found
			break
		}
		switch client.Kind() {
		case reflect.Interface, reflect.Ptr:
			client = client.Elem()
		default:
			return reflect.Value{}, fmt.Errorf(`method "%s.%s" not found`, clientName, methodName)
		}
	}
	return method, nil
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

func invoke(method reflect.Value, reqMsg proto.Message) (proto.Message, *status.Status, error) {
	ctx := context.Background()
	in := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(reqMsg),
	}
	rvalues := method.Call(in)
	if len(rvalues) != 2 {
		return nil, nil, fmt.Errorf("expected return value length of method call is 2 but %d", len(rvalues))
	}
	if !rvalues[0].IsValid() {
		return nil, nil, errors.New("first return value is invalid")
	}
	respMsg, ok := rvalues[0].Interface().(proto.Message)
	if !ok {
		if !rvalues[0].IsNil() {
			return nil, nil, fmt.Errorf("expected first return value is proto.Message but %T", rvalues[0].Interface())
		}
	}
	if !rvalues[1].IsValid() {
		return nil, nil, errors.New("second return value is invalid")
	}
	callErr, ok := rvalues[1].Interface().(error)
	if !ok {
		if !rvalues[1].IsNil() {
			return nil, nil, fmt.Errorf("expected second return value is error but %T", rvalues[1].Interface())
		}
	}
	var sts *status.Status
	if ok {
		sts, ok = status.FromError(callErr)
		if !ok {
			return nil, nil, fmt.Errorf(`expected gRPC status error but got %T: "%s"`, callErr, callErr.Error())
		}
	}
	return respMsg, sts, nil
}

func main() {}
