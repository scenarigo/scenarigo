//go:build wasip1

package plugin

import (
	"bufio"
	"bytes"
	gocontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reflect"
	"sync"
	"unsafe"

	_ "github.com/goccy/wasi-go/ext/wasip1"

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

//go:wasmimport scenarigo write
func scenarigo_write(ptr, size uint32)

//go:wasmimport scenarigo read_length
func scenarigo_read_length() uint32

//go:wasmimport scenarigo read
func scenarigo_read(uint32)

func writePluginContent(content []byte) {
	if content == nil {
		scenarigo_write(0, 0)
		return
	}
	scenarigo_write(
		uint32(uintptr(unsafe.Pointer(&content[0]))),
		uint32(len(content)),
	)
}

func readPluginContent() string {
	length := scenarigo_read_length()
	if length == 0 {
		return ""
	}
	buf := make([]byte, length)
	scenarigo_read(
		uint32(uintptr(unsafe.Pointer(&buf[0]))),
	)
	return string(buf)
}

// Register starts the WASM plugin main loop with initialization and sync functions.
// This function should be called from the main function of WASM plugins.
// initFn provides initial definitions, syncFn provides updated definitions during sync.
func Register(initFn, syncFn DefinitionFunc) {
	h := newHandler(initFn, syncFn)
	for {
		content := readPluginContent()
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
						errMsg = fatalDefaultErrorMsg
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
					writePluginContent(b)
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
			writePluginContent(b)
		}()
		wg.Wait()
	}
}

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
		typ, err := wasm.NewType(def.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to create wasm type for %s: %w", def.Name, err)
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
	ctx, err := context.FromSerializable(r.Context)
	if err != nil {
		return nil, err
	}
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
	ctx, err := context.FromSerializable(r.Context)
	if err != nil {
		return nil, err
	}
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
	ctx, err := context.FromSerializable(r.Context)
	if err != nil {
		return nil, err
	}
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
		typ, err := wasm.NewType(def.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to create wasm type for %s: %w", def.Name, err)
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
	for i := range v.Type().NumIn() {
		arg, err := wasm.DecodeValueWithType(v.Type().In(i), []byte(r.Args[i].Value))
		if err != nil {
			return nil, fmt.Errorf("failed to decode function argument: %w", err)
		}
		args = append(args, arg)
	}
	res := &wasm.CallCommandResponse{}
	for _, retValue := range v.Call(args) {
		value, err := wasm.EncodeValue(retValue)
		if err != nil {
			return nil, fmt.Errorf("failed to encode function return value: %w", err)
		}
		res.Return = append(res.Return, value)
		h.nameToValueMap[value.ID] = retValue
	}
	// Include TypeRefMap for type reference resolution on host side
	res.TypeRefMap = wasm.TypeRefMap()
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
	mtd := v.MethodByName(methodName)
	if !mtd.IsValid() {
		return nil, fmt.Errorf("failed to find method from %s", methodName)
	}
	mtdType, err := wasm.NewType(mtd)
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
	ctx, err := context.FromSerializable(r.Context)
	if err != nil {
		return nil, err
	}
	h.ctx = ctx
	var result []reflect.Value
	result = step.MethodByName("Run").Call([]reflect.Value{
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
		Context: resultCtx.ToSerializable(),
	}, nil
}

func (h *handler) LeftArrowFuncExec(r *wasm.LeftArrowFuncExecCommandRequest) (*wasm.LeftArrowFuncExecCommandResponse, error) {
	leftArrowFunc, exists := h.nameToValueMap[r.Instance]
	if !exists {
		return nil, fmt.Errorf("failed to find left arrow func instance from %s", r.Instance)
	}
	arg, exists := h.nameToValueMap[r.Value.ID]
	if !exists {
		return nil, fmt.Errorf("failed to find left arrow func argument instance from %s", r.Value.ID)
	}
	result := leftArrowFunc.MethodByName("Exec").Call([]reflect.Value{arg})
	if len(result) != 2 {
		return nil, fmt.Errorf("failed to get result value from LeftArrowFunc.Exec function. return values: %v", result)
	}
	if e := result[1].Interface(); e != nil {
		return nil, e.(error)
	}
	res, err := wasm.EncodeValue(result[0])
	if err != nil {
		return nil, err
	}
	if result[0].Kind() == reflect.Interface {
		h.nameToValueMap[res.ID] = result[0].Elem()
	} else {
		h.nameToValueMap[res.ID] = result[0]
	}
	return &wasm.LeftArrowFuncExecCommandResponse{
		Value: res,
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
	res, err := wasm.EncodeValue(result[0])
	if err != nil {
		return nil, err
	}
	h.nameToValueMap[res.ID] = result[0]
	return &wasm.LeftArrowFuncUnmarshalArgCommandResponse{
		Value: res,
	}, nil
}

func (h *handler) Get(r *wasm.GetCommandRequest) (*wasm.GetCommandResponse, error) {
	v, exists := h.nameToValueMap[r.Name]
	if !exists {
		return nil, fmt.Errorf("failed to find value: %s", r.Name)
	}
	
	// Process selectors using reflection with panic recovery
	for _, sel := range r.Selectors {
		var err error
		v, err = getFieldValueSafely(v, sel)
		if err != nil {
			return nil, err
		}
	}
	
	value, err := wasm.EncodeValue(v)
	if err != nil {
		return nil, err
	}
	// Store the encoded value in nameToValueMap for future access
	h.nameToValueMap[value.ID] = v
	return &wasm.GetCommandResponse{
		TypeRefMap: wasm.TypeRefMap(),
		Value:      value,
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

// getFieldValueSafely safely gets field value with panic recovery.
// This function handles complex selector processing on guest side.
func getFieldValueSafely(v reflect.Value, sel string) (result reflect.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic occurred while accessing field %s: %v", sel, r)
		}
	}()
	
	// Handle nil or invalid values
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("invalid value when accessing field %s", sel)
	}
	
	switch v.Type().Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Value{}, fmt.Errorf("nil pointer when accessing field %s", sel)
		}
		return getFieldValueSafely(v.Elem(), sel)
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Value{}, fmt.Errorf("nil interface when accessing field %s", sel)
		}
		return getFieldValueSafely(v.Elem(), sel)
	case reflect.Struct:
		field := v.FieldByName(sel)
		if !field.IsValid() {
			return reflect.Value{}, fmt.Errorf("field %s not found in struct type %s", sel, v.Type())
		}
		if !field.CanInterface() {
			return reflect.Value{}, fmt.Errorf("field %s is not accessible (unexported)", sel)
		}
		return field, nil
	case reflect.Map:
		key := reflect.ValueOf(sel)
		if !key.Type().AssignableTo(v.Type().Key()) {
			return reflect.Value{}, fmt.Errorf("key %s is not assignable to map key type %s", sel, v.Type().Key())
		}
		mapValue := v.MapIndex(key)
		if !mapValue.IsValid() {
			return reflect.Value{}, fmt.Errorf("key %s not found in map", sel)
		}
		return mapValue, nil
	default:
		return reflect.Value{}, fmt.Errorf("cannot access field %s on type %s (kind: %s)", sel, v.Type(), v.Type().Kind())
	}
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

func (h *handler) HTTPCall(r *wasm.HTTPCallCommandRequest) (*wasm.HTTPCallCommandResponse, error) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBuffer(r.Request)))
	if err != nil {
		return nil, err
	}
	if v := req.Header.Get(requestURLHeaderName); v != "" {
		u, err := url.Parse(v)
		if err == nil {
			req.URL = u
		}
	}
	// When http.ReadRequest is performed, a value is assigned to RequestURI.
	// However, if this field contains a value when sending the HTTP request, it results in an error.
	// Therefore, we explicitly set it to empty.
	req.RequestURI = ""

	// We remove the header that was used to store the full URL.
	req.Header.Del(requestURLHeaderName)

	// When httputil.DumpRequestOut is executed, a gzip header is added even if the Accept-Encoding header is not being used.
	// Since using Accept-Encoding complicates the response body decoding process,
	// we explicitly set it to empty in order to receive the response as uncompressed data.
	req.Header.Set("Accept-Encoding", "")

	client, exists := h.nameToValueMap[r.Client]
	if !exists {
		return nil, fmt.Errorf("unknown clientName: %q", r.Client)
	}
	do := client.MethodByName("Do")
	if !do.IsValid() {
		return nil, fmt.Errorf("failed to find Do method from client: %s", r.Client)
	}
	results := do.Call([]reflect.Value{reflect.ValueOf(req)})
	if len(results) != 2 {
		return nil, fmt.Errorf("unexpected return value length(%d) of HTTP method call", len(results))
	}
	if !results[0].IsValid() || !results[1].IsValid() {
		return nil, fmt.Errorf("HTTP response value is invalid: response:%v: error:%v", results[0], results[1])
	}
	if e := results[1].Interface(); e != nil {
		return nil, e.(error)
	}
	httpRes := results[0].Interface().(*http.Response)
	if httpRes == nil {
		return nil, errors.New("failed to get http response")
	}
	defer httpRes.Body.Close()
	resp, err := httputil.DumpResponse(httpRes, true)
	if err != nil {
		return nil, err
	}
	return &wasm.HTTPCallCommandResponse{
		Response: resp,
	}, nil
}
