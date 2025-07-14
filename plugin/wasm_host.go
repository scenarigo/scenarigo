package plugin

import (
	"bufio"
	"bytes"
	gocontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/goccy/wasi-go/imports"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	stpb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/scenarigo/scenarigo/internal/plugin/wasm"
)

var ignoreEnvNameMap = map[string]struct{}{
	// If a value greater than 1 is passed to GOMAXPROCS, a panic occurs on the plugin side,
	// so make sure not to pass it explicitly.
	"GOMAXPROCS": {},
}

func openWasmPlugin(path string) (Plugin, error) {
	ctx := gocontext.Background()
	wasmFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rcfg := wazero.NewRuntimeConfigInterpreter().
		WithCloseOnContextDone(true)

	r := wazero.NewRuntimeWithConfig(ctx, rcfg)
	compiledMod, err := r.CompileModule(ctx, wasmFile)
	if err != nil {
		return nil, err
	}

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	modCfg := wazero.NewModuleConfig().
		WithStdin(stdinR).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithFSConfig(wazero.NewFSConfig().WithFSMount(os.DirFS("/"), ""))

	srcEnvs := os.Environ()
	envs := make([]string, 0, len(srcEnvs))
	for _, kv := range srcEnvs {
		i := strings.IndexByte(kv, '=')
		key := kv[:i]
		if _, exists := ignoreEnvNameMap[key]; exists {
			continue
		}
		value := kv[i+1:]
		modCfg = modCfg.WithEnv(key, value)
		envs = append(envs, kv)
	}
	ctx, sys, err := imports.NewBuilder().
		WithSocketsExtension("wasmedgev2", compiledMod).
		WithStdio(int(stdinR.Fd()), int(os.Stdout.Fd()), int(os.Stderr.Fd())).
		WithEnv(envs...).
		WithDirs("/").
		Instantiate(ctx, r)
	if err != nil {
		return nil, err
	}
	_ = sys

	host := r.NewHostModuleBuilder("scenarigo")
	host.NewFunctionBuilder().WithGoModuleFunction(
		api.GoModuleFunc(func(ctx gocontext.Context, mod api.Module, stack []uint64) {
			b, _ := mod.Memory().Read(uint32(stack[0]), uint32(stack[1]))
			_, _ = stdoutW.Write(b)
		}),
		[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32},
		[]api.ValueType{},
	).Export("scenarigo_write")
	if _, err := host.Instantiate(ctx); err != nil {
		return nil, err
	}

	// setting the buffer size to 1 ensures that the function can exit even if there is no receiver.
	instanceModErrCh := make(chan error, 1)
	go func() {
		_, err := r.InstantiateModule(
			ctx, compiledMod, modCfg,
		)
		instanceModErrCh <- err
	}()

	plugin := &WasmPlugin{
		wasmRuntime:      r,
		stdin:            stdinW,
		stdout:           stdoutR,
		instanceModErrCh: instanceModErrCh,
	}

	res, err := plugin.call(wasm.NewInitRequest())
	if err != nil {
		return nil, err
	}
	initRes, err := convertCommandResponse[*wasm.InitCommandResponse](res)
	if err != nil {
		return nil, err
	}
	plugin.nameToTypeMap = initRes.ToTypeMap()
	return plugin, nil
}

const (
	exitCommand = "exit\n"
)

// WasmPlugin represents a WASM plugin instance.
// It manages the WASM runtime and provides communication with the WASM module.
type WasmPlugin struct {
	wasmRuntime      wazero.Runtime
	nameToTypeMap    map[string]*wasm.Type
	stdin            *os.File
	stdout           *os.File
	instanceModErrCh chan error
	instanceModErr   error
	closed           bool
	mu               sync.Mutex
}

func (p *WasmPlugin) close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	defer func() { p.closeResources(nil) }()
	if err := p.write([]byte(exitCommand)); err != nil {
		return err
	}
	return nil
}

func (p *WasmPlugin) call(req *wasm.Request) (wasm.CommandResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	b, err := wasm.EncodeRequest(req)
	if err != nil {
		return nil, err
	}
	if err := p.write(b); err != nil {
		return nil, err
	}

	resBytes, err := p.read()
	if err != nil {
		return nil, err
	}
	res, err := wasm.DecodeResponse(resBytes)
	if err != nil {
		return nil, err
	}
	if res.Error != "" {
		return nil, errors.New(res.Error)
	}
	return res.Command, nil
}

func convertCommandResponse[T wasm.CommandResponse](v wasm.CommandResponse) (T, error) {
	ret, ok := v.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("failed to convert from %T to %T", v, zero)
	}
	return ret, nil
}

func (p *WasmPlugin) write(cmd []byte) error {
	if p.closed {
		return p.instanceModErr
	}

	writeCh := make(chan error)
	go func() {
		_, err := p.stdin.Write(cmd)
		writeCh <- err
	}()
	select {
	case err := <-p.instanceModErrCh:
		// If the module instance is terminated,
		// it is considered that the termination process has been completed.
		p.closeResources(err)
		return err
	case err := <-writeCh:
		return err
	}
}

func (p *WasmPlugin) read() ([]byte, error) {
	if p.closed {
		return nil, errors.New("plugin has already been closed")
	}

	type readResult struct {
		response []byte
		err      error
	}
	readCh := make(chan readResult)
	go func() {
		reader := bufio.NewReader(p.stdout)
		content, err := reader.ReadString('\n')
		if err != nil {
			readCh <- readResult{err: fmt.Errorf("failed to receive response from wasm plugin: %w", err)}
			return
		}
		if content == "" {
			readCh <- readResult{err: errors.New("receive empty response from wasm plugin")}
			return
		}
		readCh <- readResult{response: []byte(content)}
	}()
	select {
	case err := <-p.instanceModErrCh:
		// If the module instance is terminated,
		// it is considered that the termination process has been completed.
		p.closeResources(err)
		return nil, err
	case result := <-readCh:
		return result.response, result.err
	}
}

func (p *WasmPlugin) closeResources(instanceModErr error) {
	p.instanceModErr = instanceModErr
	p.closed = true
	p.stdin.Close()
	p.stdout.Close()
}

func (p *WasmPlugin) Lookup(name string) (Symbol, error) {
	return nil, fmt.Errorf("symbol %q not found", name)
}

func (p *WasmPlugin) GetSetup() SetupFunc {
	return func(sctx *Context) (*Context, func(*Context)) {
		ctx, teardown, err := p.setup(sctx)
		if err != nil {
			panic(err)
		}
		return ctx, teardown
	}
}

func (p *WasmPlugin) setup(sctx *Context) (*Context, func(*Context), error) {
	setupID := fmt.Sprintf("%p", sctx)
	if _, err := p.call(wasm.NewSetupRequest(setupID, sctx.ToSerializable())); err != nil {
		return nil, nil, err
	}
	res, err := p.call(wasm.NewSyncRequest())
	if err != nil {
		return nil, nil, err
	}
	syncRes, err := convertCommandResponse[*wasm.SyncCommandResponse](res)
	if err != nil {
		return nil, nil, err
	}
	p.nameToTypeMap = syncRes.ToTypeMap()
	return sctx, func(sctx *Context) {
		if _, err := p.call(wasm.NewTeardownRequest(setupID, sctx.ToSerializable())); err != nil {
			panic(err)
		}
		if err := p.close(); err != nil {
			panic(err)
		}
	}, nil
}

func (p *WasmPlugin) GetSetupEachScenario() SetupFunc {
	return func(sctx *Context) (*Context, func(*Context)) {
		ctx, teardown, err := p.setupEachScenario(sctx)
		if err != nil {
			panic(err)
		}
		return ctx, teardown
	}
}

func (p *WasmPlugin) setupEachScenario(sctx *Context) (*Context, func(*Context), error) {
	setupID := fmt.Sprintf("%p", sctx)
	if _, err := p.call(wasm.NewSetupEachScenarioRequest(setupID, sctx.ToSerializable())); err != nil {
		return nil, nil, err
	}
	res, err := p.call(wasm.NewSyncRequest())
	if err != nil {
		return nil, nil, err
	}
	syncRes, err := convertCommandResponse[*wasm.SyncCommandResponse](res)
	if err != nil {
		return nil, nil, err
	}
	p.nameToTypeMap = syncRes.ToTypeMap()
	return sctx, func(sctx *Context) {
		if _, err := p.call(wasm.NewTeardownRequest(setupID, sctx.ToSerializable())); err != nil {
			panic(err)
		}
	}, nil
}

// ExtractByKey implements query.KeyExtractor interface.
func (p *WasmPlugin) ExtractByKey(name string) (any, bool) {
	typ, exists := p.nameToTypeMap[name]
	if !exists {
		return nil, false
	}
	ret, err := p.getValue(typ, name, nil)
	if err != nil {
		panic(err)
	}
	return ret, true
}

func (p *WasmPlugin) callFunc(typ *wasm.FuncType, name string, args []reflect.Value) ([]reflect.Value, error) {
	fnArgs := make([]string, 0, len(args))
	for _, arg := range args {
		b, err := json.Marshal(arg.Interface())
		if err != nil {
			return nil, err
		}
		fnArgs = append(fnArgs, string(b))
	}

	res, err := p.call(wasm.NewCallRequest(name, fnArgs))
	if err != nil {
		return nil, err
	}
	funcRes, err := convertCommandResponse[*wasm.CallCommandResponse](res)
	if err != nil {
		return nil, err
	}
	if len(funcRes.Return) != len(typ.Return) {
		return nil, fmt.Errorf("expected function return value num is %d but got %d", len(typ.Return), len(funcRes.Return))
	}

	ret := make([]reflect.Value, 0, len(typ.Return))
	for idx, retValue := range typ.Return {
		if retValue.IsStruct() {
			ret = append(ret, reflect.ValueOf(
				&StructValue{
					typ:    retValue,
					plugin: p,
					name:   funcRes.Return[idx].ID,
					value:  funcRes.Return[idx].Value,
				},
			))
			continue
		}
		typ, err := retValue.ToReflect()
		if err != nil {
			return nil, err
		}
		rv := reflect.New(typ)
		v := rv.Interface()
		if err := json.Unmarshal([]byte(funcRes.Return[idx].Value), v); err != nil {
			return nil, fmt.Errorf("failed to convert function return value(%d) %s to %s: %w", idx, funcRes.Return[idx].Value, typ.Kind(), err)
		}
		ret = append(ret, rv.Elem())
	}
	return ret, nil
}

func (p *WasmPlugin) getValue(typ *wasm.Type, name string, selectors []string) (any, error) {
	if typ.Kind == wasm.INVALID {
		return nil, nil
	}
	if typ.HasMethod(name) {
		// method call.
		mtdType := typ.MethodTypeByName(name)
		funcType, err := mtdType.ToReflect()
		if err != nil {
			return nil, err
		}
		fn := reflect.MakeFunc(funcType, func(args []reflect.Value) []reflect.Value {
			ret, err := p.callFunc(mtdType, name, args)
			if err != nil {
				panic(err)
			}
			return ret
		})
		return fn.Interface(), nil
	}
	if typ.Kind == wasm.FUNC {
		funcType, err := typ.ToReflect()
		if err != nil {
			return nil, err
		}
		fn := reflect.MakeFunc(funcType, func(args []reflect.Value) []reflect.Value {
			ret, err := p.callFunc(typ.Func, name, args)
			if err != nil {
				panic(err)
			}
			return ret
		})
		return fn.Interface(), nil
	}
	res, err := p.call(wasm.NewGetRequest(name, selectors))
	if err != nil {
		return nil, err
	}
	valRes, err := convertCommandResponse[*wasm.GetCommandResponse](res)
	if err != nil {
		return nil, err
	}
	t, err := typ.ToReflect()
	if err != nil {
		return nil, err
	}
	for _, sel := range selectors {
		var err error
		t, err = getFieldTypeByName(t, sel)
		if err != nil {
			return nil, err
		}
		typ = typ.FieldTypeByName(sel)
	}
	if typ.IsStruct() {
		return &StructValue{
			typ:       typ,
			name:      name,
			selectors: selectors,
			plugin:    p,
		}, nil
	}
	rv := reflect.New(t)
	if err := json.Unmarshal([]byte(valRes.Value), rv.Interface()); err != nil {
		return nil, err
	}
	return rv.Elem().Interface(), nil
}

func getFieldTypeByName(t reflect.Type, sel string) (reflect.Type, error) {
	switch t.Kind() {
	case reflect.Pointer, reflect.Interface:
		return getFieldTypeByName(t.Elem(), sel)
	case reflect.Struct:
		sf, exists := t.FieldByName(sel)
		if !exists {
			return nil, fmt.Errorf("failed to find field %s type from %v", sel, t)
		}
		return sf.Type, nil
	}
	return nil, fmt.Errorf("failed to get field %s type from %v", sel, t)
}

// StructValue represents a struct type value from a WASM plugin.
// It provides methods to interact with WASM plugin values, especially for gRPC clients.
type StructValue struct {
	typ       *wasm.Type
	plugin    *WasmPlugin
	name      string
	selectors []string
	value     any
}

// ExtractByKey implements query.KeyExtractor interface for StructValue.
// It always returns false as WASM values don't support key extraction.
func (v *StructValue) ExtractByKey(key string) (any, bool) {
	value, err := v.plugin.getValue(
		v.typ,
		v.name,
		append(append([]string{}, v.selectors...), key),
	)
	if err != nil {
		panic(err)
	}
	return value, true
}

// ExistsMethod checks if a gRPC method exists on the WASM value.
// This is used for gRPC client validation in WASM plugins.
func (v *StructValue) ExistsMethod(method string) bool {
	res, err := v.plugin.call(wasm.NewGRPCExistsMethodRequest(v.name, method))
	if err != nil {
		return false
	}
	conv, err := convertCommandResponse[*wasm.GRPCExistsMethodCommandResponse](res)
	if err != nil {
		return false
	}
	return conv.Exists
}

// BuildRequestMessage builds a protobuf request message for a gRPC method.
// This is used to construct request messages for gRPC calls in WASM plugins.
func (v *StructValue) BuildRequestMessage(method string, msg []byte) (proto.Message, error) {
	res, err := v.plugin.call(wasm.NewGRPCBuildRequestRequest(v.name, method, msg))
	if err != nil {
		return nil, err
	}
	buildRes, err := convertCommandResponse[*wasm.GRPCBuildRequestCommandResponse](res)
	if err != nil {
		return nil, err
	}

	var fdset descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(buildRes.FDSet, &fdset); err != nil {
		return nil, err
	}
	files, err := protodesc.NewFiles(&fdset)
	if err != nil {
		return nil, err
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(buildRes.MessageFQDN))
	if err != nil {
		return nil, err
	}
	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("failed to convert message descriptor from %T", desc)
	}
	protoMsg := dynamicpb.NewMessage(msgDesc)
	if !bytes.Equal(msg, []byte(`null`)) {
		if err := protojson.Unmarshal(msg, protoMsg); err != nil {
			return nil, err
		}
	}
	return protoMsg, nil
}

// Invoke calls a gRPC method on the WASM value with the given request.
// It returns the response message, status, and any error that occurred.
func (v *StructValue) Invoke(method string, reqProto proto.Message) (proto.Message, *status.Status, error) {
	reqMsg, err := protojson.Marshal(reqProto)
	if err != nil {
		return nil, nil, err
	}
	res, err := v.plugin.call(wasm.NewGRPCInvokeRequest(v.name, method, reqMsg))
	if err != nil {
		return nil, nil, err
	}
	invokeRes, err := convertCommandResponse[*wasm.GRPCInvokeCommandResponse](res)
	if err != nil {
		return nil, nil, err
	}

	var fdset descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(invokeRes.FDSet, &fdset); err != nil {
		return nil, nil, err
	}
	files, err := protodesc.NewFiles(&fdset)
	if err != nil {
		return nil, nil, err
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(invokeRes.ResponseFQDN))
	if err != nil {
		return nil, nil, err
	}
	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, nil, fmt.Errorf("failed to convert message descriptor from %T", desc)
	}
	protoMsg := dynamicpb.NewMessage(msgDesc)
	if !bytes.Equal(invokeRes.ResponseBytes, []byte(`null`)) {
		if err := protojson.Unmarshal(invokeRes.ResponseBytes, protoMsg); err != nil {
			return nil, nil, err
		}
	}
	var stProto stpb.Status
	if err := proto.Unmarshal(invokeRes.StatusProto, &stProto); err != nil {
		return nil, nil, err
	}
	st := status.FromProto(&stProto)
	return protoMsg, st, nil
}
