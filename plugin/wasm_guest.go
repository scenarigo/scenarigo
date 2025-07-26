//go:build wasip1

package plugin

import (
	"bufio"
	gocontext "context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"
	"unsafe"

	_ "github.com/goccy/wasi-go-net/wasip1"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/plugin"
	"github.com/scenarigo/scenarigo/internal/plugin/wasm"
)

var (
	setups             []SetupFunc
	setupsEachScenario []SetupFunc
)

// SetupFunc represents a setup function for WASM plugins.
// If it returns non-nil teardown, the function will be called later during cleanup.
type SetupFunc func(ctx *Context) (newCtx *Context, teardown func(*Context))

// RegisterSetup registers a function to setup for WASM plugin.
// WASM plugins must call this function in their init function to register the setup process.
func RegisterSetup(setup SetupFunc) {
	setups = append(setups, setup)
}

// RegisterSetupEachScenario registers a function to setup for WASM plugin.
// WASM plugins must call this function in their init function to register the setup process.
// The registered function will be called before each scenario.
func RegisterSetupEachScenario(setup SetupFunc) {
	setupsEachScenario = append(setupsEachScenario, setup)
}

// Definition represents a named value definition for WASM plugins.
// It contains the name and reflection value of functions or variables exported by the plugin.
type Definition struct {
	Name  string
	Value reflect.Value
}

// ToDefinition creates a Definition from a name and value.
// This function is used by WASM plugins to define exported functions and variables.
func ToDefinition(name string, value any) *Definition {
	return &Definition{Name: name, Value: reflect.ValueOf(value)}
}

// DefinitionFunc is a function type that returns a list of definitions.
// It is used by WASM plugins to provide their exported functions and variables.
type DefinitionFunc func() []*Definition

// Register starts the WASM plugin main loop with initialization and sync functions.
// This function should be called from the main function of WASM plugins.
// initFn provides initial definitions, syncFn provides updated definitions during sync.
func Register(initFn, syncFn DefinitionFunc) {
	reader := bufio.NewReader(os.Stdin)
	h := newHandler(initFn, syncFn)
	for {
		content, err := reader.ReadString('\n')
		if err != nil {
			continue
		}
		if content == "" {
			continue
		}
		if content == exitCommand {
			return
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			var finished bool
			defer func() {
				if !finished {
					var errMsg string
					if r := recover(); r != nil {
						errMsg = fmt.Sprint(r)
					} else {
						errMsg = "plugin executed panic(nil) or runtime.Goexit"
					}
					var sctx *context.SerializableContext
					if h.ctx != nil {
						sctx = h.ctx.ToSerializable()
					}
					var req wasm.Request
					_ = json.Unmarshal([]byte(content), &req)
					b, _ := json.Marshal(&wasm.Response{
						CommandType: req.CommandType,
						Context:     sctx,
						Error:       errMsg,
					})
					out := append(b, '\n')
					scenarigo_write(
						uint32(uintptr(unsafe.Pointer(&out[0]))),
						uint32(len(out)),
					)
				}
				wg.Done()
			}()
			h.ctx = nil
			res := wasm.HandleCommand([]byte(content), h)
			if h.ctx != nil {
				res.Context = h.ctx.ToSerializable()
			}
			finished = true
			b, _ := json.Marshal(res)
			out := append(b, '\n')
			scenarigo_write(
				uint32(uintptr(unsafe.Pointer(&out[0]))),
				uint32(len(out)),
			)
		}()
		wg.Wait()
	}
}

//go:wasmimport scenarigo scenarigo_write
func scenarigo_write(ptr, size uint32)

type handler struct {
	ctx                *Context
	initFn             DefinitionFunc
	syncFn             DefinitionFunc
	teardownMap        map[string]func(*Context)
	funcNameToValueMap map[string]reflect.Value
	nameToValueMap     map[string]reflect.Value
}

func newHandler(initFn, syncFn DefinitionFunc) *handler {
	return &handler{
		initFn:             initFn,
		syncFn:             syncFn,
		teardownMap:        make(map[string]func(*Context)),
		funcNameToValueMap: make(map[string]reflect.Value),
		nameToValueMap:     make(map[string]reflect.Value),
	}
}

func (h *handler) Init(r *wasm.InitCommandRequest) (*wasm.InitCommandResponse, error) {
	var types []*wasm.NameWithType
	for _, def := range h.initFn() {
		if !def.Value.IsValid() {
			types = append(types, &wasm.NameWithType{
				Name: def.Name,
				Type: &wasm.Type{Kind: wasm.INVALID},
			})
			continue
		}
		h.nameToValueMap[def.Name] = def.Value
		typ, err := wasm.NewType(def.Value.Type())
		if err != nil {
			return nil, err
		}
		types = append(types, &wasm.NameWithType{
			Name: def.Name,
			Type: typ,
		})
	}
	return &wasm.InitCommandResponse{
		SetupNum:             len(setups),
		SetupEachScenarioNum: len(setupsEachScenario),
		TypeRefMap:           wasm.TypeRefMap(),
		Types:                types,
	}, nil
}

func (h *handler) Setup(r *wasm.SetupCommandRequest) (*wasm.SetupCommandResponse, error) {
	ctx := r.ToContext()
	setup := setups[r.Idx]
	h.ctx = ctx
	ctx, teardown := setup(ctx)
	if ctx != nil {
		h.ctx = ctx
	}
	h.teardownMap[r.ID] = teardown
	return &wasm.SetupCommandResponse{
		ExistsTeardown: teardown != nil,
	}, nil
}

func (h *handler) SetupEachScenario(r *wasm.SetupEachScenarioCommandRequest) (*wasm.SetupEachScenarioCommandResponse, error) {
	ctx := r.ToContext()
	setup := setupsEachScenario[r.Idx]
	h.ctx = ctx
	ctx, teardown := setup(ctx)
	if ctx != nil {
		h.ctx = ctx
	}
	h.teardownMap[r.ID] = teardown
	return &wasm.SetupEachScenarioCommandResponse{
		ExistsTeardown: teardown != nil,
	}, nil
}

func (h *handler) Teardown(r *wasm.TeardownCommandRequest) (*wasm.TeardownCommandResponse, error) {
	ctx := r.ToContext()
	teardown := h.teardownMap[r.ID]
	h.ctx = ctx
	teardown(ctx)
	return &wasm.TeardownCommandResponse{}, nil
}

func (h *handler) Sync(r *wasm.SyncCommandRequest) (*wasm.SyncCommandResponse, error) {
	defs := h.syncFn()
	for _, def := range defs {
		h.nameToValueMap[def.Name] = def.Value
	}
	var types []*wasm.NameWithType
	for _, def := range defs {
		if !def.Value.IsValid() {
			types = append(types, &wasm.NameWithType{
				Name: def.Name,
				Type: &wasm.Type{Kind: wasm.INVALID},
			})
			continue
		}
		h.nameToValueMap[def.Name] = def.Value
		typ, err := wasm.NewType(def.Value.Type())
		if err != nil {
			return nil, err
		}
		types = append(types, &wasm.NameWithType{
			Name: def.Name,
			Type: typ,
		})
	}
	return &wasm.SyncCommandResponse{
		TypeRefMap: wasm.TypeRefMap(), Types: types,
	}, nil
}

var (
	stepType          = reflect.TypeOf((*Step)(nil)).Elem()
	leftArrowFuncType = reflect.TypeOf((*LeftArrowFunc)(nil)).Elem()
)

func (h *handler) Call(r *wasm.CallCommandRequest) (*wasm.CallCommandResponse, error) {
	v, exists := h.nameToValueMap[r.Name]
	if !exists {
		return nil, fmt.Errorf("failed to find function: %s", r.Name)
	}
	if len(r.Selectors) != 0 {
		// method call.
		for _, sel := range r.Selectors[:len(r.Selectors)-1] {
			var err error
			v, err = getFieldValue(v, sel)
			if err != nil {
				return nil, err
			}
		}
		methodName := r.Selectors[len(r.Selectors)-1]
		v = v.MethodByName(methodName)
	}
	if len(r.Args) != v.Type().NumIn() {
		return nil, fmt.Errorf(
			"expected %s function argument number is %d but got %d",
			r.Name,
			v.Type().NumIn(),
			len(r.Args),
		)
	}
	args := make([]reflect.Value, 0, v.Type().NumIn())
	for i := 0; i < v.Type().NumIn(); i++ {
		rv := reflect.New(v.Type().In(i))
		arg := rv.Interface()
		if err := json.Unmarshal([]byte(r.Args[i]), arg); err != nil {
			return nil, fmt.Errorf(
				"failed to decode %s function argument(%d): %s",
				r.Name,
				i,
				r.Args[i],
			)
		}
		args = append(args, rv.Elem())
	}
	res := &wasm.CallCommandResponse{}
	for i, retValue := range v.Call(args) {
		typ := retValue.Type()
		var value string
		switch {
		case typ == stepType:
			value = stepInterface
		case typ == leftArrowFuncType:
			value = leftArrowFuncInterface
		default:
			b, err := json.Marshal(retValue.Interface())
			if err != nil {
				return nil, fmt.Errorf(
					"failed to encode %s function return value (%d): %v",
					r.Name,
					i,
					retValue.Interface(),
				)
			}
			value = string(b)
		}
		valueID := fmt.Sprintf("%p", &retValue)
		h.nameToValueMap[valueID] = retValue
		res.Return = append(res.Return, &wasm.ReturnValue{
			ID:    valueID,
			Value: value,
		})
	}
	return res, nil
}

func (h *handler) Method(r *wasm.MethodCommandRequest) (*wasm.MethodCommandResponse, error) {
	v, exists := h.nameToValueMap[r.Name]
	if !exists {
		return nil, fmt.Errorf("failed to find function: %s", r.Name)
	}
	for _, sel := range r.Selectors[:len(r.Selectors)-1] {
		var err error
		v, err = getFieldValue(v, sel)
		if err != nil {
			return nil, err
		}
	}
	methodName := r.Selectors[len(r.Selectors)-1]
	mtd, exists := v.Type().MethodByName(methodName)
	if !exists {
		return nil, fmt.Errorf("failed to find method from %s", methodName)
	}
	mtdType, err := wasm.NewType(mtd.Type)
	if err != nil {
		return nil, err
	}
	if mtdType.Kind != wasm.FUNC {
		return nil, fmt.Errorf("failed to create method type: %s", mtdType)
	}
	return &wasm.MethodCommandResponse{
		TypeRefMap: wasm.TypeRefMap(),
		Type:       mtdType,
	}, nil
}

func (h *handler) StepRun(r *wasm.StepRunCommandRequest) (res *wasm.StepRunCommandResponse, e error) {
	step, exists := h.nameToValueMap[r.Instance]
	if !exists {
		return nil, fmt.Errorf("failed to find step instance from %s", r.Instance)
	}
	ctx := context.FromSerializable(r.Context)
	h.ctx = ctx
	result := step.MethodByName("Run").Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(r.Step),
	})
	if len(result) != 1 {
		return nil, fmt.Errorf("failed to get result value from step.Run function. return values: %v", result)
	}
	resultCtx, ok := result[0].Interface().(*context.Context)
	if !ok {
		return nil, fmt.Errorf("failed to convert result type to *context.Context from %T", result[0].Interface())
	}
	return &wasm.StepRunCommandResponse{
		Context:        resultCtx.ToSerializable(),
		IsSpawnContext: ctx != resultCtx,
	}, nil
}

func (h *handler) LeftArrowFuncExec(r *wasm.LeftArrowFuncExecCommandRequest) (*wasm.LeftArrowFuncExecCommandResponse, error) {
	leftArrowFunc, exists := h.nameToValueMap[r.Instance]
	if !exists {
		return nil, fmt.Errorf("failed to find left arrow func instance from %s", r.Instance)
	}
	argValue, exists := h.nameToValueMap[r.ArgID]
	if !exists {
		return nil, fmt.Errorf("failed to find left arrow func argument instance from %s", r.ArgID)
	}
	var v any
	if err := json.Unmarshal([]byte(r.Value), &v); err != nil {
		return nil, err
	}
	result := leftArrowFunc.MethodByName("Exec").Call([]reflect.Value{argValue})
	if len(result) != 2 {
		return nil, fmt.Errorf("failed to get result value from LeftArrowFunc.Exec function. return values: %v", result)
	}
	if e := result[1].Interface(); e != nil {
		return nil, e.(error)
	}
	res, err := json.Marshal(result[0].Interface())
	if err != nil {
		return nil, err
	}
	return &wasm.LeftArrowFuncExecCommandResponse{
		Value: string(res),
	}, nil
}

func (h *handler) LeftArrowFuncUnmarshalArg(r *wasm.LeftArrowFuncUnmarshalArgCommandRequest) (*wasm.LeftArrowFuncUnmarshalArgCommandResponse, error) {
	leftArrowFunc, exists := h.nameToValueMap[r.Instance]
	if !exists {
		return nil, fmt.Errorf("failed to find left arrow func instance from %s", r.Instance)
	}
	result := leftArrowFunc.MethodByName("UnmarshalArg").Call([]reflect.Value{
		reflect.ValueOf(func(v any) error {
			if err := yaml.Unmarshal([]byte(r.Value), v); err != nil {
				return err
			}
			return nil
		}),
	})
	if len(result) != 2 {
		return nil, fmt.Errorf("failed to get result value from LeftArrowFunc.UnmarshalArg function. return values: %v", result)
	}
	if e := result[1].Interface(); e != nil {
		return nil, e.(error)
	}
	value := result[0]
	valueID := fmt.Sprintf("%p", value)
	h.nameToValueMap[valueID] = value
	res, err := json.Marshal(value.Interface())
	if err != nil {
		return nil, err
	}
	return &wasm.LeftArrowFuncUnmarshalArgCommandResponse{
		ValueID: valueID,
		Value:   string(res),
	}, nil
}

func (h *handler) Get(r *wasm.GetCommandRequest) (*wasm.GetCommandResponse, error) {
	v, exists := h.nameToValueMap[r.Name]
	if !exists {
		return nil, fmt.Errorf("failed to find value: %s", r.Name)
	}
	for _, sel := range r.Selectors {
		var err error
		v, err = getFieldValue(v, sel)
		if err != nil {
			return nil, err
		}
	}
	b, err := json.Marshal(v.Interface())
	if err != nil {
		return nil, err
	}
	return &wasm.GetCommandResponse{
		Value: string(b),
	}, nil
}

func getFieldValue(v reflect.Value, sel string) (reflect.Value, error) {
	switch v.Type().Kind() {
	case reflect.Pointer, reflect.Interface:
		return getFieldValue(v.Elem(), sel)
	case reflect.Struct:
		return v.FieldByName(sel), nil
	}
	return reflect.Value{}, fmt.Errorf("failed to get field %s value from %v", sel, v)
}

func (h *handler) GRPCExistsMethod(r *wasm.GRPCExistsMethodCommandRequest) (*wasm.GRPCExistsMethodCommandResponse, error) {
	method, err := h.getMethod(r.Client, r.Method)
	if err != nil {
		return nil, err
	}
	if err := plugin.ValidateGRPCMethod(method); err != nil {
		return nil, fmt.Errorf(`"%s.%s" must be "func(context.Context, proto.Message, ...grpc.CallOption) (proto.Message, error): %w"`, r.Client, r.Method, err)
	}
	return &wasm.GRPCExistsMethodCommandResponse{
		Exists: true,
	}, nil
}

func (h *handler) GRPCBuildRequest(r *wasm.GRPCBuildRequestCommandRequest) (*wasm.GRPCBuildRequestCommandResponse, error) {
	method, err := h.getMethod(r.Client, r.Method)
	if err != nil {
		return nil, err
	}
	reqProtoMsg, err := h.requestProtoMessage(method)
	if err != nil {
		return nil, err
	}
	fdSetBytes, err := h.currentFileDescriptorSetBytes()
	if err != nil {
		return nil, err
	}
	return &wasm.GRPCBuildRequestCommandResponse{
		MessageFQDN: string(reqProtoMsg.ProtoReflect().Descriptor().FullName()),
		FDSet:       fdSetBytes,
	}, nil
}

func (h *handler) GRPCInvoke(r *wasm.GRPCInvokeCommandRequest) (*wasm.GRPCInvokeCommandResponse, error) {
	method, err := h.getMethod(r.Client, r.Method)
	if err != nil {
		return nil, err
	}
	reqProtoMsg, err := h.requestProtoMessage(method)
	if err != nil {
		return nil, err
	}
	if err := protojson.Unmarshal(r.Request, reqProtoMsg); err != nil {
		return nil, err
	}
	ctx := metadata.NewOutgoingContext(gocontext.Background(), r.Metadata)
	resMsg, st, err := plugin.GRPCInvoke(ctx, method, reqProtoMsg)
	if err != nil {
		return nil, err
	}
	resMsgBytes, err := protojson.Marshal(resMsg)
	if err != nil {
		return nil, err
	}
	fdSetBytes, err := h.currentFileDescriptorSetBytes()
	if err != nil {
		return nil, err
	}
	stBytes, err := proto.Marshal(st.Proto())
	if err != nil {
		return nil, err
	}
	return &wasm.GRPCInvokeCommandResponse{
		FDSet:         fdSetBytes,
		ResponseFQDN:  string(resMsg.ProtoReflect().Descriptor().FullName()),
		ResponseBytes: resMsgBytes,
		StatusProto:   stBytes,
	}, nil
}

func (h *handler) requestProtoMessage(method reflect.Value) (proto.Message, error) {
	reqMsg := reflect.New(method.Type().In(1).Elem()).Interface()
	ret, ok := reqMsg.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to convert to request message: %T", reqMsg)
	}
	return ret, nil
}

func (h *handler) currentFileDescriptorSetBytes() ([]byte, error) {
	var fdSet descriptorpb.FileDescriptorSet
	protoregistry.GlobalFiles.RangeFiles(func(desc protoreflect.FileDescriptor) bool {
		fd := protodesc.ToFileDescriptorProto(desc)
		fdSet.File = append(fdSet.File, fd)
		return true
	})
	b, err := proto.Marshal(&fdSet)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (h *handler) getMethod(clientName, methodName string) (reflect.Value, error) {
	client, exists := h.nameToValueMap[clientName]
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
