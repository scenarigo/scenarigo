//go:build wasip1

package plugin

import (
	"bufio"
	gocontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"plugin"
	"reflect"
	"runtime"
	"strconv"
	"unsafe"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	_ "github.com/goccy/wasi-go-net/wasip1"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

// Symbol is a pointer to a variable or function.
type Symbol = plugin.Symbol

// SetupFunc represents a setup function.
// If it returns non-nil teardown, the function will be called later.
type SetupFunc func(ctx *Context) (newCtx *Context, teardown func(*Context))

// Plugin represents a scenarigo plugin.
type Plugin interface {
	Lookup(name string) (Symbol, error)
	GetSetup() SetupFunc
	GetSetupEachScenario() SetupFunc
}

var (
	setups             []SetupFunc
	setupsEachScenario []SetupFunc
	teardownMap        = make(map[string][]func(*Context))
)

// RegisterSetup registers a function to setup for plugin.
// Plugins must call this function in their init function if it registers the setup process.
func RegisterSetup(setup SetupFunc) {
	setups = append(setups, setup)
}

// RegisterSetupEachScenario registers a function to setup for plugin.
// Plugins must call this function in their init function if it registers the setup process.
// The registered function will be called before each scenario.
func RegisterSetupEachScenario(setup SetupFunc) {
	setupsEachScenario = append(setupsEachScenario, setup)
}

type Definition struct {
	Name  string
	Value reflect.Value
}

func ToDefinition(name string, value any) *Definition {
	return &Definition{Name: name, Value: reflect.ValueOf(value)}
}

//go:wasmimport scenarigo scenarigo_write
func scenarigo_write(ptr, size uint32)

type InitFunc func() []*Definition
type RegisterValuesFunc func() []*Definition

func Register(initFn InitFunc, registerValuesFn RegisterValuesFunc) {
	reader := bufio.NewReader(os.Stdin)
	for {
		content, err := reader.ReadString('\n')
		if err != nil {
			continue
		}
		if content == "" {
			continue
		}
		if content == "exit\n" {
			return
		}
		if content == "gc\n" {
			runtime.GC()
			continue
		}
		body, err := handlePluginRequest([]byte(content), initFn, registerValuesFn)
		var res *PluginResponse
		if err != nil {
			res = &PluginResponse{Error: err.Error()}
		} else {
			res = &PluginResponse{Body: body}
		}
		b, _ := json.Marshal(res)
		out := append(b, '\n')
		scenarigo_write(
			uint32(uintptr(unsafe.Pointer(&out[0]))),
			uint32(len(out)),
		)
	}
}

func handlePluginRequest(r []byte, initFn InitFunc, registerValuesFn RegisterValuesFunc) (any, error) {
	var req PluginRequest
	if err := json.Unmarshal(r, &req); err != nil {
		return nil, err
	}
	switch req.Method {
	case "init":
		return runInit(initFn()...)
	case "setup":
		var v SetupRequest
		if err := json.Unmarshal(req.Request, &v); err != nil {
			return nil, err
		}
		return runSetup(&v)
	case "setup_each_scenario":
		var v SetupRequest
		if err := json.Unmarshal(req.Request, &v); err != nil {
			return nil, err
		}
		return runSetupEachScenario(&v)
	case "teardown":
		var v TeardownRequest
		if err := json.Unmarshal(req.Request, &v); err != nil {
			return nil, err
		}
		return runTeardown(&v)
	case "register_values":
		return registerValues(registerValuesFn()...)
	case "call":
		var v FuncCall
		if err := json.Unmarshal(req.Request, &v); err != nil {
			return nil, err
		}
		return callFunction(&v)
	case "get":
		var v GetValueRequest
		if err := json.Unmarshal(req.Request, &v); err != nil {
			return nil, err
		}
		return getValue(&v)
	case "grpc_client_exists_method":
		var v ExistsMethodRequest
		if err := json.Unmarshal(req.Request, &v); err != nil {
			return nil, err
		}
		return grpcClientExistsMethod(&v)
	case "grpc_client_build_request":
		var v BuildRequest
		if err := json.Unmarshal(req.Request, &v); err != nil {
			return nil, err
		}
		return grpcClientBuildRequest(&v)
	case "grpc_client_invoke":
		var v InvokeRequest
		if err := json.Unmarshal(req.Request, &v); err != nil {
			return nil, err
		}
		return grpcClientInvoke(&v)
	}
	return nil, fmt.Errorf("unexpected method: %s", req.Method)
}

func runInit(defs ...*Definition) (*InitResponse, error) {
	var types []*NameWithType
	for _, def := range defs {
		if !def.Value.IsValid() {
			types = append(types, &NameWithType{
				Name: def.Name,
				Type: &Type{Kind: reflect.Invalid},
			})
			continue
		}
		if def.Value.Type().Kind() == reflect.Func {
			registerFunc(def.Name, def.Value)
		} else {
			idToValueMap[def.Name] = def.Value
		}
		types = append(types, &NameWithType{
			Name: def.Name,
			Type: toType(def.Value.Type()),
		})
	}
	return &InitResponse{Types: types}, nil
}

func registerValues(defs ...*Definition) (*InitResponse, error) {
	for _, def := range defs {
		idToValueMap[def.Name] = def.Value
	}
	var types []*NameWithType
	for _, def := range defs {
		if !def.Value.IsValid() {
			types = append(types, &NameWithType{
				Name: def.Name,
				Type: &Type{Kind: reflect.Invalid},
			})
			continue
		}
		idToValueMap[def.Name] = def.Value
		types = append(types, &NameWithType{
			Name: def.Name,
			Type: toType(def.Value.Type()),
		})
	}
	return &InitResponse{Types: types}, nil
}

func runSetup(req *SetupRequest) ([]byte, error) {
	ctx := context.FromSerializable(req.Context)
	for i, setup := range setups {
		newCtx := ctx
		ctx.Run(strconv.Itoa(i+1), func(ctx *Context) {
			ctx, teardown := setup(ctx)
			if ctx != nil {
				newCtx = ctx
			}
			if teardown != nil {
				teardownMap[req.ID] = append(teardownMap[req.ID], teardown)
			}
		})
		ctx = newCtx.WithReporter(ctx.Reporter())
	}
	return nil, nil
}

func runSetupEachScenario(req *SetupRequest) ([]byte, error) {
	ctx := context.FromSerializable(req.Context)
	for i, setup := range setupsEachScenario {
		newCtx := ctx
		ctx.Run(strconv.Itoa(i+1), func(ctx *Context) {
			ctx, teardown := setup(ctx)
			if ctx != nil {
				newCtx = ctx
			}
			if teardown != nil {
				teardownMap[req.ID] = append(teardownMap[req.ID], teardown)
			}
		})
		ctx = newCtx.WithReporter(ctx.Reporter())
	}
	return nil, nil
}

func runTeardown(req *TeardownRequest) ([]byte, error) {
	ctx := context.FromSerializable(req.Context)
	for i, teardown := range teardownMap[req.SetupID] {
		ctx.Run(strconv.Itoa(i+1), func(ctx *Context) {
			teardown(ctx)
		})
	}
	return nil, nil
}

var funcNameToValueMap = make(map[string]reflect.Value)

var idToValueMap = make(map[string]reflect.Value)

func callFunction(req *FuncCall) (*CallFunctionResponse, error) {
	v, exists := funcNameToValueMap[req.Name]
	if !exists {
		return nil, fmt.Errorf("failed to find function: %s", req.Name)
	}
	if len(req.Args) != v.Type().NumIn() {
		return nil, fmt.Errorf(
			"expected %s function argument number is %d but got %d",
			req.Name,
			v.Type().NumIn(),
			len(req.Args),
		)
	}
	args := make([]reflect.Value, 0, v.Type().NumIn())
	for i := 0; i < v.Type().NumIn(); i++ {
		rv := reflect.New(v.Type().In(i))
		arg := rv.Interface()
		if err := json.Unmarshal([]byte(req.Args[i]), arg); err != nil {
			return nil, fmt.Errorf(
				"failed to decode %s function argument(%d): %s",
				req.Name,
				i,
				req.Args[i],
			)
		}
		args = append(args, rv.Elem())
	}

	res := &CallFunctionResponse{}
	for i, retValue := range v.Call(args) {
		b, err := json.Marshal(retValue.Interface())
		if err != nil {
			return nil, fmt.Errorf(
				"failed to encode %s function return value (%d): %v",
				req.Name,
				i,
				retValue.Interface(),
			)
		}
		valueID := fmt.Sprintf("%p", &retValue)
		idToValueMap[valueID] = retValue
		res.Return = append(res.Return, &ReturnValue{
			ID:    valueID,
			Value: string(b),
		})
	}
	return res, nil
}

func getValue(req *GetValueRequest) (*GetValueResponse, error) {
	v, exists := idToValueMap[req.Name]
	if !exists {
		return nil, fmt.Errorf("failed to find value: %s", req.Name)
	}
	b, err := json.Marshal(v.Interface())
	if err != nil {
		return nil, err
	}
	return &GetValueResponse{Value: string(b)}, nil
}

func registerFunc(name string, v reflect.Value) {
	funcNameToValueMap[name] = v
}

func grpcClientExistsMethod(req *ExistsMethodRequest) (*ExistsMethodResponse, error) {
	method, err := getMethod(req.ClientName, req.MethodName)
	if err != nil {
		return nil, err
	}
	if err := validateMethod(method); err != nil {
		return nil, fmt.Errorf(`"%s.%s" must be "func(context.Context, proto.Message, ...grpc.CallOption) (proto.Message, error): %w"`, req.ClientName, req.MethodName, err)
	}
	return &ExistsMethodResponse{
		Exists: true,
	}, nil
}

func grpcClientBuildRequest(req *BuildRequest) (*BuildResponse, error) {
	method, err := getMethod(req.ClientName, req.MethodName)
	if err != nil {
		return nil, err
	}
	reqMsg := reflect.New(method.Type().In(1).Elem()).Interface()
	if _, ok := reqMsg.(proto.Message); !ok {
		return nil, fmt.Errorf("failed to convert to request message: %T", reqMsg)
	}
	var fdSet descriptorpb.FileDescriptorSet
	protoregistry.GlobalFiles.RangeFiles(func(fileDescriptor protoreflect.FileDescriptor) bool {
		fdProto := protodesc.ToFileDescriptorProto(fileDescriptor)
		fdSet.File = append(fdSet.File, fdProto)
		return true
	})
	fdSetBytes, err := proto.Marshal(&fdSet)
	if err != nil {
		return nil, err
	}
	return &BuildResponse{
		MessageFQDN: string(reqMsg.(proto.Message).ProtoReflect().Descriptor().FullName()),
		FDSet:       fdSetBytes,
	}, nil
}

func grpcClientInvoke(req *InvokeRequest) (*InvokeResponse, error) {
	method, err := getMethod(req.ClientName, req.MethodName)
	if err != nil {
		return nil, err
	}
	reqMsg := reflect.New(method.Type().In(1).Elem()).Interface()
	if _, ok := reqMsg.(proto.Message); !ok {
		return nil, fmt.Errorf("failed to convert to request message: %T", reqMsg)
	}
	reqProtoMsg := reqMsg.(proto.Message)
	if err := protojson.Unmarshal(req.Request, reqProtoMsg); err != nil {
		return nil, err
	}
	resMsg, st, err := invoke(method, reqProtoMsg)
	if err != nil {
		return nil, err
	}
	resMsgBytes, err := protojson.Marshal(resMsg)
	if err != nil {
		return nil, err
	}
	var fdSet descriptorpb.FileDescriptorSet
	protoregistry.GlobalFiles.RangeFiles(func(fileDescriptor protoreflect.FileDescriptor) bool {
		fdProto := protodesc.ToFileDescriptorProto(fileDescriptor)
		fdSet.File = append(fdSet.File, fdProto)
		return true
	})
	fdSetBytes, err := proto.Marshal(&fdSet)
	if err != nil {
		return nil, err
	}
	stBytes, err := proto.Marshal(st.Proto())
	if err != nil {
		return nil, err
	}
	return &InvokeResponse{
		FDSet:         fdSetBytes,
		ResponseFQDN:  string(resMsg.ProtoReflect().Descriptor().FullName()),
		ResponseBytes: resMsgBytes,
		StatusProto:   stBytes,
	}, nil
}

func getMethod(clientName, methodName string) (reflect.Value, error) {
	client, exists := idToValueMap[clientName]
	if !exists {
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
	typeContext  = reflect.TypeOf((*gocontext.Context)(nil)).Elem()
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
	ctx := gocontext.Background()
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
