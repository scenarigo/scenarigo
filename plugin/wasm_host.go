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
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/goccy/wasi-go/imports"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	stpb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/plugin/wasm"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
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
		WithWasiGoExtension().
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

	plugin := &WasmPlugin{
		wasmRuntime: r,
		stdin:       stdinW,
		stdout:      stdoutR,
	}

	// setting the buffer size to 1 ensures that the function can exit even if there is no receiver.
	instanceModErrCh := make(chan error, 1)
	go func() {
		_, err := r.InstantiateModule(
			ctx, compiledMod, modCfg,
		)
		instanceModErrCh <- err
		plugin.closeResources(err)
	}()

	plugin.instanceModErrCh = instanceModErrCh

	res, err := plugin.call(nil, wasm.NewInitRequest())
	if err != nil {
		return nil, err
	}
	initRes, err := convertCommandResponse[*wasm.InitCommandResponse](res)
	if err != nil {
		return nil, err
	}
	typeMap, err := initRes.ToTypeMap()
	if err != nil {
		return nil, err
	}
	plugin.setupNum = initRes.SetupNum
	plugin.setupEachScenarioNum = initRes.SetupEachScenarioNum
	plugin.nameToTypeMap = typeMap
	return plugin, nil
}

const (
	exitCommand = "exit\n"
)

// WasmPlugin represents a WASM plugin instance.
// It manages the WASM runtime and provides communication with the WASM module.
type WasmPlugin struct {
	wasmRuntime          wazero.Runtime
	nameToTypeMap        map[string]*wasm.Type
	setupNum             int
	setupEachScenarioNum int
	stdin                *os.File
	stdout               *os.File
	instanceModErrCh     chan error
	instanceModErr       error
	closed               bool
	mu                   sync.Mutex
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

func (p *WasmPlugin) call(ctx *Context, req *wasm.Request) (wasm.CommandResponse, error) {
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
	if res != nil && res.Context != nil && res.Context.ReporterID != "" {
		// Logs and other records made through the reporter when executed in the plugin are reflected in the current reporter.
		curReporter, ok := ctx.Reporter().(interface {
			SetFromSerializable(string, map[string]*reporter.SerializableReporter)
		})
		if ok {
			curReporter.SetFromSerializable(res.Context.ReporterID, res.Context.ReporterMap)
		}
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
	return p.getSetup(p.setupNum, func(ctx *Context, idx int) (*Context, func(*Context), error) {
		return p.setup(ctx, idx)
	})
}

func (p *WasmPlugin) GetSetupEachScenario() SetupFunc {
	return p.getSetup(p.setupEachScenarioNum, func(ctx *Context, idx int) (*Context, func(*Context), error) {
		return p.setupEachScenario(ctx, idx)
	})
}

func (p *WasmPlugin) getSetup(setupNum int, setupCallback func(*Context, int) (*Context, func(*Context), error)) SetupFunc {
	if setupNum == 0 {
		return nil
	}
	if setupNum == 1 {
		return func(sctx *Context) (*Context, func(*Context)) {
			ctx, teardown, err := setupCallback(sctx, 0)
			if err != nil {
				ctx.Reporter().Fatal(err)
			}
			return ctx, teardown
		}
	}
	return func(ctx *Context) (*Context, func(*Context)) {
		var teardowns []func(*Context)
		for i := range setupNum {
			newCtx := ctx
			ctx.Run(strconv.Itoa(i+1), func(ctx *Context) {
				ctx, teardown, err := setupCallback(ctx, i)
				if err != nil {
					return
				}
				if ctx != nil {
					newCtx = ctx
				}
				if teardown != nil {
					teardowns = append(teardowns, teardown)
				}
			})
			ctx = newCtx.WithReporter(ctx.Reporter())
		}
		if len(teardowns) == 0 {
			return ctx, nil
		}
		if len(teardowns) == 1 {
			return ctx, teardowns[0]
		}
		return ctx, func(ctx *Context) {
			for i, teardown := range teardowns {
				ctx.Run(strconv.Itoa(i+1), func(ctx *Context) {
					teardown(ctx)
				})
			}
		}
	}
}

func (p *WasmPlugin) setup(sctx *Context, idx int) (*Context, func(*Context), error) {
	id := fmt.Sprintf("%p%d", sctx, idx)
	setupBaseRes, err := p.call(sctx, wasm.NewSetupRequest(id, sctx.ToSerializable(), idx))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to setup: %w", err)
	}
	setupRes, err := convertCommandResponse[*wasm.SetupCommandResponse](setupBaseRes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert setup response: %w", err)
	}
	res, err := p.call(sctx, wasm.NewSyncRequest())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sync: %w", err)
	}
	syncRes, err := convertCommandResponse[*wasm.SyncCommandResponse](res)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert sync response: %w", err)
	}
	typeMap, err := syncRes.ToTypeMap()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get type map: %w", err)
	}
	for k, v := range typeMap {
		p.nameToTypeMap[k] = v
	}
	if !setupRes.ExistsTeardown {
		return sctx, nil, nil
	}
	return sctx, func(sctx *Context) {
		// ignore teardown process's error.
		_, _ = p.call(sctx, wasm.NewTeardownRequest(id, sctx.ToSerializable()))
		_ = p.close()
	}, nil
}

func (p *WasmPlugin) setupEachScenario(sctx *Context, idx int) (*Context, func(*Context), error) {
	id := fmt.Sprintf("%p%d", sctx, idx)
	setupBaseRes, err := p.call(sctx, wasm.NewSetupEachScenarioRequest(id, sctx.ToSerializable(), idx))
	if err != nil {
		return nil, nil, err
	}
	setupRes, err := convertCommandResponse[*wasm.SetupEachScenarioCommandResponse](setupBaseRes)
	if err != nil {
		return nil, nil, err
	}
	res, err := p.call(sctx, wasm.NewSyncRequest())
	if err != nil {
		return nil, nil, err
	}
	syncRes, err := convertCommandResponse[*wasm.SyncCommandResponse](res)
	if err != nil {
		return nil, nil, err
	}
	typeMap, err := syncRes.ToTypeMap()
	if err != nil {
		return nil, nil, err
	}
	for k, v := range typeMap {
		p.nameToTypeMap[k] = v
	}
	if !setupRes.ExistsTeardown {
		return sctx, nil, nil
	}
	return sctx, func(sctx *Context) {
		// ignore teardown process's error.
		_, _ = p.call(sctx, wasm.NewTeardownRequest(id, sctx.ToSerializable()))
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

func (p *WasmPlugin) callFunc(typ *wasm.FuncType, name string, selectors []string, args []reflect.Value) ([]reflect.Value, error) {
	fnArgs := make([]*wasm.Value, 0, len(args))
	for _, arg := range args {
		fnArg, err := wasm.EncodeValue(arg)
		if err != nil {
			return nil, err
		}
		fnArgs = append(fnArgs, fnArg)
	}

	res, err := p.call(nil, wasm.NewCallRequest(name, selectors, fnArgs))
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
		value := funcRes.Return[idx]
		v, err := p.decodeValue(retValue, value)
		if err != nil {
			return nil, err
		}
		ret = append(ret, v)
	}
	return ret, nil
}

func (p *WasmPlugin) getValue(typ *wasm.Type, name string, selectors []string) (any, error) {
	if typ.Kind == wasm.INVALID {
		fqdn := strings.Join(append([]string{name}, selectors...), ".")
		return nil, fmt.Errorf("%s: invalid type", fqdn)
	}
	if len(selectors) != 0 {
		for _, sel := range selectors[:len(selectors)-1] {
			typ = typ.FieldTypeByName(sel)
		}
		lastSel := selectors[len(selectors)-1]
		if typ.HasMethod(lastSel) {
			// method call.
			res, err := p.call(nil, wasm.NewMethodRequest(name, selectors))
			if err != nil {
				return nil, err
			}
			mtdRes, err := convertCommandResponse[*wasm.MethodCommandResponse](res)
			if err != nil {
				return nil, err
			}
			mtdType, err := wasm.ResolveRef(mtdRes.Type, mtdRes.TypeRefMap)
			if err != nil {
				return nil, err
			}
			mtdType.Func.Args = mtdType.Func.Args[1:]
			funcType, err := mtdType.ToReflect()
			if err != nil {
				return nil, err
			}
			fn := reflect.MakeFunc(replaceStructType(funcType), func(args []reflect.Value) []reflect.Value {
				ret, err := p.callFunc(mtdType.Func, name, selectors, args)
				if err != nil {
					panic(err)
				}
				return ret
			})
			return fn.Interface(), nil
		}
		typ = typ.FieldTypeByName(lastSel)
	}
	if typ.Kind == wasm.FUNC {
		if typ.Step {
			return &StructValue{
				typ:    typ,
				plugin: p,
				name:   name,
			}, nil
		} else if typ.StepFunc {
			return &StructValue{
				typ:    typ,
				plugin: p,
				name:   name,
			}, nil
		}
		funcType, err := typ.ToReflect()
		if err != nil {
			return nil, err
		}
		fn := reflect.MakeFunc(replaceStructType(funcType), func(args []reflect.Value) []reflect.Value {
			ret, err := p.callFunc(typ.Func, name, selectors, args)
			if err != nil {
				panic(err)
			}
			return ret
		})
		return fn.Interface(), nil
	}
	if typ.IsStruct() {
		return &StructValue{
			typ:       typ,
			name:      name,
			selectors: selectors,
			plugin:    p,
		}, nil
	}
	res, err := p.call(nil, wasm.NewGetRequest(name, selectors))
	if err != nil {
		return nil, err
	}
	valRes, err := convertCommandResponse[*wasm.GetCommandResponse](res)
	if err != nil {
		return nil, err
	}
	v, err := p.decodeValue(typ, valRes.Value)
	if err != nil {
		return nil, err
	}
	return v.Interface(), nil
}

var ctxType = reflect.TypeOf((*Context)(nil))

func replaceStructType(t reflect.Type) reflect.Type {
	if t == ctxType {
		return t
	}
	if wasm.IsStepFuncType(t) {
		return reflect.TypeOf(&StructValue{})
	}
	switch t.Kind() {
	case reflect.Pointer:
		return reflect.New(replaceStructType(t.Elem())).Type()
	case reflect.Map:
		return reflect.MapOf(replaceStructType(t.Key()), replaceStructType(t.Elem()))
	case reflect.Slice:
		return reflect.SliceOf(replaceStructType(t.Elem()))
	case reflect.Array:
		return reflect.ArrayOf(t.Len(), replaceStructType(t.Elem()))
	case reflect.Func:
		args := make([]reflect.Type, 0, t.NumIn())
		for i := range t.NumIn() {
			args = append(args, replaceStructType(t.In(i)))
		}
		ret := make([]reflect.Type, 0, t.NumOut())
		for i := range t.NumOut() {
			ret = append(ret, replaceStructType(t.Out(i)))
		}
		return reflect.FuncOf(args, ret, false)
	case reflect.Struct:
		return reflect.TypeOf(StructValue{})
	}
	return t
}

// StructValue represents a struct type value from a WASM plugin.
// It provides methods to interact with WASM plugin values, especially for gRPC clients.
type StructValue struct {
	typ       *wasm.Type
	plugin    *WasmPlugin
	name      string
	selectors []string
	value     any
	argID     string
}

func (v *StructValue) Exec(arg any) (any, error) {
	if !v.typ.LeftArrowFunc {
		return nil, fmt.Errorf("%s doesn't implement plugin.LeftArrowFunc", v.name)
	}
	value, err := wasm.EncodeValue(reflect.ValueOf(arg))
	if err != nil {
		return nil, err
	}
	res, err := v.plugin.call(nil, wasm.NewLeftArrowFuncExecRequest(v.name, value.Value, v.argID))
	if err != nil {
		return nil, err
	}
	execRes, err := convertCommandResponse[*wasm.LeftArrowFuncExecCommandResponse](res)
	if err != nil {
		return nil, err
	}
	result, err := v.plugin.decodeValue(execRes.Value.Type, execRes.Value)
	if err != nil {
		return nil, err
	}
	return result.Interface(), nil
}

func (v *StructValue) UnmarshalArg(unmarshal func(any) error) (any, error) {
	if !v.typ.LeftArrowFunc {
		return nil, fmt.Errorf("%s doesn't implement plugin.LeftArrowFunc", v.name)
	}
	var decoded any
	if err := unmarshal(&decoded); err != nil {
		return nil, err
	}
	b, err := yaml.Marshal(decoded)
	if err != nil {
		return nil, err
	}
	res, err := v.plugin.call(nil, wasm.NewLeftArrowFuncUnmarshalArgRequest(v.name, string(b)))
	if err != nil {
		return nil, err
	}
	argRes, err := convertCommandResponse[*wasm.LeftArrowFuncUnmarshalArgCommandResponse](res)
	if err != nil {
		return nil, err
	}
	result, err := v.plugin.decodeValue(argRes.Value.Type, argRes.Value)
	if err != nil {
		return nil, err
	}
	v.argID = argRes.Value.ID
	return result.Interface(), nil
}

const fatalDefaultErrorMsg = "plugin executed panic(nil) or runtime.Goexit"

func (v *StructValue) Run(ctx *Context, step *schema.Step) *Context {
	if !v.typ.Step && !v.typ.StepFunc {
		ctx.Reporter().Fatal(fmt.Errorf("%s doesn't implement plugin.Step", v.name))
	}
	res, err := v.plugin.call(ctx, wasm.NewStepRunRequest(v.name, ctx.ToSerializable(), step))
	if err != nil {
		if e := err.Error(); e != fatalDefaultErrorMsg {
			ctx.Reporter().Error(err)
		}
		ctx.Reporter().FailNow()
		return ctx
	}
	stepRes, err := convertCommandResponse[*wasm.StepRunCommandResponse](res)
	if err != nil {
		ctx.Reporter().Fatal(err)
	}
	newCtx, err := context.FromSerializableWithContext(ctx, stepRes.Context)
	if err != nil {
		ctx.Reporter().Fatal(err)
	}
	return newCtx
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
	res, err := v.plugin.call(nil, wasm.NewGRPCExistsMethodRequest(v.name, method))
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
	res, err := v.plugin.call(nil, wasm.NewGRPCBuildRequestRequest(v.name, method, msg))
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
func (v *StructValue) Invoke(ctx gocontext.Context, method string, reqProto proto.Message) (proto.Message, *status.Status, error) {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}
	reqMsg, err := protojson.Marshal(reqProto)
	if err != nil {
		return nil, nil, err
	}
	res, err := v.plugin.call(nil, wasm.NewGRPCInvokeRequest(v.name, method, reqMsg, md))
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

const requestURLHeaderName = "X-Wasm-Plugin-Request-Url"

func (v *StructValue) Do(req *http.Request) (*http.Response, error) {
	// When httputil.DumpRequestOut is executed, the full URL information including the scheme is lost.
	// Therefore, we explicitly save the entire URL in a header so that the guest side can reconstruct the URL.
	// This header will be removed on the guest side before sending the request.
	req.Header.Set(requestURLHeaderName, req.URL.String())
	r, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		return nil, err
	}
	res, err := v.plugin.call(nil, wasm.NewHTTPCallRequest(v.name, r))
	if err != nil {
		return nil, err
	}
	httpCallRes, err := convertCommandResponse[*wasm.HTTPCallCommandResponse](res)
	if err != nil {
		return nil, err
	}
	resp, err := http.ReadResponse(
		bufio.NewReader(bytes.NewBuffer(httpCallRes.Response)),
		req,
	)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *WasmPlugin) decodeValue(typ *wasm.Type, v *wasm.Value) (reflect.Value, error) {
	if v.Type.Kind == wasm.ERROR {
		var errText string
		_ = json.Unmarshal([]byte(v.Value), &errText)
		if errText != "" {
			return reflect.Value{}, errors.New(errText)
		}
		return reflect.Zero(reflect.TypeOf((*error)(nil)).Elem()), nil
	}
	if v.Type.Step || v.Type.StepFunc || v.Type.LeftArrowFunc || v.Type.IsStruct() {
		return reflect.ValueOf(
			&StructValue{
				typ:    v.Type,
				plugin: p,
				name:   v.ID,
				value:  v.Value,
			},
		), nil
	}
	rtyp, err := typ.ToReflect()
	if err != nil {
		return reflect.Value{}, err
	}
	rv, err := wasm.DecodeValueWithType(rtyp, []byte(v.Value))
	if err != nil {
		return reflect.Value{}, fmt.Errorf("failed to convert value %s to %s: %w", v.Value, rtyp.Kind(), err)
	}
	if rv.Kind() == reflect.Interface {
		kind := rv.Elem().Kind()
		if kind == reflect.Map || kind == reflect.Struct {
			return reflect.ValueOf(
				&StructValue{
					typ:    typ,
					plugin: p,
					name:   v.ID,
					value:  v.Value,
				},
			), nil
		}
	}
	return rv, nil
}
