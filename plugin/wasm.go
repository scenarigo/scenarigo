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

	"github.com/scenarigo/scenarigo/context"
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
		WithFSConfig(
			wazero.NewFSConfig().WithFSMount(os.DirFS("/"), ""),
		)

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
			//nolint:gosec
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

	var v InitResponse
	if err := plugin.call(&PluginRequest{
		Method: "init",
	}, &v); err != nil {
		return nil, err
	}
	nameToTypeMap := make(map[string]*Type)
	for _, typ := range v.Types {
		nameToTypeMap[typ.Name] = typ.Type
	}
	plugin.nameToTypeMap = nameToTypeMap
	return plugin, nil
}

type WasmPlugin struct {
	wasmRuntime      wazero.Runtime
	nameToTypeMap    map[string]*Type
	stdin            *os.File
	stdout           *os.File
	instanceModErrCh chan error
	instanceModErr   error
	closed           bool
	mu               sync.Mutex
}

type PluginRequest struct {
	Method  string `json:"method"`
	Request []byte `json:"request"`
}

type PluginResponse struct {
	Error string `json:"error"`
	Body  any    `json:"body"`
}

func (p *WasmPlugin) call(req *PluginRequest, res any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := p.write(append(reqBytes, '\n')); err != nil {
		return err
	}

	resBytes, err := p.read()
	if err != nil {
		return err
	}
	var pluginRes PluginResponse
	if err := json.Unmarshal(resBytes, &pluginRes); err != nil {
		return err
	}
	if pluginRes.Error != "" {
		return errors.New(pluginRes.Error)
	}
	body, err := json.Marshal(pluginRes.Body)
	if err != nil {
		return err
	}
	if res != nil {
		if err := json.Unmarshal(body, res); err != nil {
			return err
		}
	}
	return nil
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

type InitResponse struct {
	Types []*NameWithType `json:"types"`
}

type NameWithType struct {
	Name string `json:"name"`
	Type *Type  `json:"type"`
}

type FuncType struct {
	Args   []*Type `json:"args"`
	Return []*Type `json:"return"`
}

func (t *FuncType) toReflectType() reflect.Type {
	args := make([]reflect.Type, 0, len(t.Args))
	for _, arg := range t.Args {
		args = append(args, arg.toReflectType())
	}
	ret := make([]reflect.Type, 0, len(t.Return))
	for _, r := range t.Return {
		ret = append(ret, r.toReflectType())
	}
	return reflect.FuncOf(args, ret, false)
}

type Type struct {
	Kind reflect.Kind `json:"kind"`
	Func *FuncType    `json:"func"`
}

func (t *Type) toReflectType() reflect.Type {
	if t.Func != nil {
		return t.Func.toReflectType()
	}
	switch t.Kind {
	case reflect.Bool:
		return reflect.TypeOf(false)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return reflect.TypeOf(0)
	case reflect.String:
		return reflect.TypeOf("")
	case reflect.Array, reflect.Slice:
	}
	return reflect.TypeOf(&wasmValue{})
}

func (p *WasmPlugin) Lookup(name string) (Symbol, error) {
	fmt.Println("called Lookup", name)
	return nil, nil
}

type SetupRequest struct {
	ID      string                       `json:"id"`
	Context *context.SerializableContext `json:"context"`
}

type TeardownRequest struct {
	SetupID string                       `json:"setupId"`
	Context *context.SerializableContext `json:"context"`
}

func (p *WasmPlugin) GetSetup() SetupFunc {
	return func(sctx *Context) (*Context, func(*Context)) {
		sc := sctx.ToSerializable()
		setupID := fmt.Sprintf("%p", sc)
		setupReq, err := json.Marshal(&SetupRequest{
			ID:      setupID,
			Context: sc,
		})
		if err != nil {
			panic(err)
		}
		if err := p.call(&PluginRequest{
			Method:  "setup",
			Request: setupReq,
		}, nil); err != nil {
			panic(err)
		}
		var v InitResponse
		if err := p.call(&PluginRequest{
			Method: "register_values",
		}, &v); err != nil {
			panic(err)
		}
		for _, typ := range v.Types {
			p.nameToTypeMap[typ.Name] = typ.Type
		}
		return sctx, func(sctx *Context) {
			teardownReq, err := json.Marshal(&TeardownRequest{
				SetupID: setupID,
				Context: sctx.ToSerializable(),
			})
			if err != nil {
				panic(err)
			}
			if err := p.call(&PluginRequest{
				Method:  "teardown",
				Request: teardownReq,
			}, nil); err != nil {
				panic(err)
			}
		}
	}
}

func (p *WasmPlugin) GetSetupEachScenario() SetupFunc {
	return func(sctx *Context) (*Context, func(*Context)) {
		sc := sctx.ToSerializable()
		setupID := fmt.Sprintf("%p", sc)
		setupReq, err := json.Marshal(&SetupRequest{
			ID:      setupID,
			Context: sc,
		})
		if err != nil {
			panic(err)
		}
		if err := p.call(&PluginRequest{
			Method:  "setup_each_scenario",
			Request: setupReq,
		}, nil); err != nil {
			panic(err)
		}
		var v InitResponse
		if err := p.call(&PluginRequest{
			Method: "register_values",
		}, &v); err != nil {
			panic(err)
		}
		for _, typ := range v.Types {
			p.nameToTypeMap[typ.Name] = typ.Type
		}
		return sctx, func(sctx *Context) {
			teardownReq, err := json.Marshal(&TeardownRequest{
				SetupID: setupID,
				Context: sctx.ToSerializable(),
			})
			if err != nil {
				panic(err)
			}
			if err := p.call(&PluginRequest{
				Method:  "teardown",
				Request: teardownReq,
			}, nil); err != nil {
				panic(err)
			}
		}
	}
}

type FuncCall struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
}

func toType(typ reflect.Type) *Type {
	if typ.Kind() == reflect.Func {
		return toFuncType(typ)
	}
	return &Type{Kind: typ.Kind()}
}

func toFuncType(typ reflect.Type) *Type {
	fn := &FuncType{}
	for i := 0; i < typ.NumIn(); i++ {
		fn.Args = append(fn.Args, toType(typ.In(i)))
	}
	for i := 0; i < typ.NumOut(); i++ {
		fn.Return = append(fn.Return, toType(typ.Out(i)))
	}
	return &Type{Kind: typ.Kind(), Func: fn}
}

// ExtractByKey implements query.KeyExtractor interface.
func (p *WasmPlugin) ExtractByKey(name string) (any, bool) {
	typ, exists := p.nameToTypeMap[name]
	if !exists {
		return nil, false
	}
	if typ.Kind == reflect.Invalid {
		return nil, true
	}
	if typ.Func != nil {
		funcType := typ.Func.toReflectType()
		fn := reflect.MakeFunc(funcType, func(args []reflect.Value) []reflect.Value {
			call := &FuncCall{Name: name}
			for _, arg := range args {
				b, err := json.Marshal(arg.Interface())
				if err != nil {
					panic(err)
				}
				call.Args = append(call.Args, string(b))
			}
			callReq, err := json.Marshal(call)
			if err != nil {
				panic(err)
			}
			var funcRes CallFunctionResponse
			if err := p.call(&PluginRequest{
				Method:  "call",
				Request: callReq,
			}, &funcRes); err != nil {
				panic(err)
			}
			if funcRes.Error != "" {
				panic(funcRes.Error)
			}
			if len(funcRes.Return) != len(typ.Func.Return) {
				panic(
					fmt.Sprintf(
						"expected function return value num is %d but got %d",
						len(typ.Func.Return),
						len(funcRes.Return),
					),
				)
			}
			ret := make([]reflect.Value, 0, len(typ.Func.Return))
			for idx, retValue := range typ.Func.Return {
				typ := retValue.toReflectType()
				rv := reflect.New(typ)
				v := rv.Interface()
				if wv, ok := v.(**wasmValue); ok {
					*wv = &wasmValue{
						plugin:    p,
						valueName: funcRes.Return[idx].ID,
						value:     funcRes.Return[idx].Value,
					}
					ret = append(ret, rv.Elem())
					continue
				}
				if err := json.Unmarshal([]byte(funcRes.Return[idx].Value), v); err != nil {
					panic(
						fmt.Sprintf(
							"failed to convert function return value(%d) %s to %s: %v",
							idx,
							funcRes.Return[idx].Value,
							typ.Kind(),
							err,
						),
					)
				}
				ret = append(ret, rv.Elem())
			}
			return ret
		})
		return fn.Interface(), true
	}

	getReq, err := json.Marshal(&GetValueRequest{Name: name})
	if err != nil {
		panic(err)
	}
	var valRes GetValueResponse
	if err := p.call(&PluginRequest{
		Method:  "get",
		Request: getReq,
	}, &valRes); err != nil {
		panic(err)
	}
	if valRes.Error != "" {
		panic(valRes.Error)
	}
	rv := reflect.New(typ.toReflectType())
	if err := json.Unmarshal([]byte(valRes.Value), rv.Interface()); err != nil {
		panic(err)
	}
	ret := rv.Elem().Interface()
	if v, ok := ret.(*wasmValue); ok {
		v.valueName = name
		v.plugin = p
	}
	return ret, true
}

type wasmValue struct {
	plugin    *WasmPlugin
	valueName string
	value     any
}

func (v *wasmValue) ExtractByKey(key string) (any, bool) {
	fmt.Println("called wasmValue", key)
	return nil, false
}

type ReturnValue struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type GetValueRequest struct {
	Name string `json:"name"`
}

type GetValueResponse struct {
	Value string `json:"value"`
	Error string `json:"error"`
}

type CallFunctionResponse struct {
	Return []*ReturnValue `json:"return"`
	Error  string         `json:"error"`
}

type ExistsMethodRequest struct {
	ClientName string `json:"clientName"`
	MethodName string `json:"methodName"`
}

type ExistsMethodResponse struct {
	Exists bool   `json:"exists"`
	Error  string `json:"error"`
}

func (v *wasmValue) ExistsMethod(method string) bool {
	req, err := json.Marshal(&ExistsMethodRequest{
		ClientName: v.valueName,
		MethodName: method,
	})
	if err != nil {
		return false
	}
	var res ExistsMethodResponse
	if err := v.plugin.call(&PluginRequest{
		Method:  "grpc_client_exists_method",
		Request: req,
	}, &res); err != nil {
		return false
	}
	return res.Exists
}

type BuildRequest struct {
	ClientName string `json:"clientName"`
	MethodName string `json:"methodName"`
	Message    []byte `json:"message"`
}

type BuildResponse struct {
	FDSet       []byte `json:"fdset"`
	MessageFQDN string `json:"messageFQDN"`
	Error       string `json:"error"`
}

func (v *wasmValue) BuildRequestMessage(method string, msg []byte) (proto.Message, error) {
	req, err := json.Marshal(&BuildRequest{
		ClientName: v.valueName,
		MethodName: method,
		Message:    msg,
	})
	if err != nil {
		return nil, err
	}
	var res BuildResponse
	if err := v.plugin.call(&PluginRequest{
		Method:  "grpc_client_build_request",
		Request: req,
	}, &res); err != nil {
		return nil, err
	}
	var fdset descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(res.FDSet, &fdset); err != nil {
		return nil, err
	}
	files, err := protodesc.NewFiles(&fdset)
	if err != nil {
		return nil, err
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(res.MessageFQDN))
	if err != nil {
		return nil, err
	}
	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("failed to convert message descriptor from %T", desc)
	}
	protoMsg := dynamicpb.NewMessage(msgDesc)
	if !bytes.Equal(msg, []byte("null")) {
		if err := protojson.Unmarshal(msg, protoMsg); err != nil {
			return nil, err
		}
	}
	return protoMsg, nil
}

type InvokeRequest struct {
	ClientName string `json:"clientName"`
	MethodName string `json:"methodName"`
	Request    []byte `json:"request"`
}

type InvokeResponse struct {
	FDSet         []byte `json:"fdset"`
	ResponseFQDN  string `json:"responseFQDN"`
	ResponseBytes []byte `json:"responseBytes"`
	StatusProto   []byte `json:"statusProto"`
	Error         string `json:"error"`
}

func (v *wasmValue) Invoke(method string, reqProto proto.Message) (proto.Message, *status.Status, error) {
	reqMsg, err := protojson.Marshal(reqProto)
	if err != nil {
		return nil, nil, err
	}
	req, err := json.Marshal(&InvokeRequest{
		ClientName: v.valueName,
		MethodName: method,
		Request:    reqMsg,
	})
	if err != nil {
		return nil, nil, err
	}
	var res InvokeResponse
	if err := v.plugin.call(&PluginRequest{
		Method:  "grpc_client_invoke",
		Request: req,
	}, &res); err != nil {
		return nil, nil, err
	}
	var fdset descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(res.FDSet, &fdset); err != nil {
		return nil, nil, err
	}
	files, err := protodesc.NewFiles(&fdset)
	if err != nil {
		return nil, nil, err
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(res.ResponseFQDN))
	if err != nil {
		return nil, nil, err
	}
	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, nil, fmt.Errorf("failed to convert message descriptor from %T", desc)
	}
	protoMsg := dynamicpb.NewMessage(msgDesc)
	if !bytes.Equal(res.ResponseBytes, []byte("null")) {
		if err := protojson.Unmarshal(res.ResponseBytes, protoMsg); err != nil {
			return nil, nil, err
		}
	}
	var stProto stpb.Status
	if err := proto.Unmarshal(res.StatusProto, &stProto); err != nil {
		return nil, nil, err
	}
	st := status.FromProto(&stProto)
	return protoMsg, st, nil
}
