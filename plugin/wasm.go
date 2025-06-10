package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

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
)

var ignoreEnvNameMap = map[string]struct{}{
	// If a value greater than 1 is passed to GOMAXPROCS, a panic occurs on the plugin side,
	// so make sure not to pass it explicitly.
	"GOMAXPROCS": {},
}

func openWasmPlugin(path string) (Plugin, error) {
	fmt.Println("openWasmPlugin", path)
	ctx := context.Background()
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

	srcEnvs := os.Environ()
	envs := make([]string, 0, len(srcEnvs))
	for _, kv := range srcEnvs {
		i := strings.IndexByte(kv, '=')
		key := kv[:i]
		if _, exists := ignoreEnvNameMap[key]; exists {
			continue
		}
		envs = append(envs, kv)
	}
	ctx, sys, err := imports.NewBuilder().
		WithSocketsExtension("wasmedgev2", compiledMod).
		WithStdio(int(os.Stdin.Fd()), int(os.Stdout.Fd()), int(os.Stderr.Fd())).
		WithEnv(envs...).
		WithDirs("/").
		Instantiate(ctx, r)
	_ = sys

	mod, err := r.InstantiateModule(
		ctx,
		compiledMod,
		wazero.NewModuleConfig().
			WithStartFunctions("_initialize").
			WithName("scenarigo"),
	)
	if err != nil {
		return nil, err
	}
	bytes, err := callExportedFunction(ctx, mod, "__scenarigo_plugin_init", nil)
	var v ExportedValue
	if err := json.Unmarshal(bytes, &v); err != nil {
		return nil, err
	}
	return &wasmPlugin{
		ctx:           ctx,
		mod:           mod,
		wasmRuntime:   r,
		exportedValue: &v,
	}, nil
}

type ExportedValue struct {
	GRPCClients []string `json:"grpc_clients"`
}

type wasmPlugin struct {
	ctx           context.Context
	mod           api.Module
	wasmRuntime   wazero.Runtime
	exportedValue *ExportedValue
}

func (p *wasmPlugin) Lookup(name string) (Symbol, error) {
	fmt.Println("called Lookup", name)
	return nil, nil
}

func (p *wasmPlugin) GetSetup() SetupFunc {
	return func(sctx *Context) (*Context, func(*Context)) {
		encoded := sctx.ToSerializable()
		b, err := json.Marshal(encoded)
		if err != nil {
			fmt.Println(err)
			return nil, nil
		}
		addr, err := p.mod.ExportedFunction("__alloc").Call(p.ctx, uint64(len(b)))
		if err != nil {
			fmt.Println(err)
			return nil, nil
		}
		if ok := p.mod.Memory().Write(uint32(addr[0]), b); !ok {
			fmt.Println("not ok")
			return nil, nil
		}
		if _, err := callExportedFunction(
			p.ctx,
			p.mod,
			"__scenarigo_plugin_setup",
			[]uint64{
				addr[0],
				uint64(len(b)),
			},
		); err != nil {
			fmt.Println("err", err)
			return nil, nil
		}
		return sctx, func(sctx *Context) {
			encoded := sctx.ToSerializable()
			b, err := json.Marshal(encoded)
			if err != nil {
				fmt.Println(err)
			}
			addr, err := p.mod.ExportedFunction("__alloc").Call(p.ctx, uint64(len(b)))
			if err != nil {
				fmt.Println(err)
				return
			}
			if ok := p.mod.Memory().Write(uint32(addr[0]), b); !ok {
				fmt.Println("not ok")
				return
			}
			_, _ = callExportedFunction(
				p.ctx,
				p.mod,
				"__scenarigo_plugin_teardown",
				[]uint64{
					addr[0],
					uint64(len(b)),
				},
			)
		}
	}
}

func (p *wasmPlugin) GetSetupEachScenario() SetupFunc {
	fmt.Println("called GetSetupEachScenario")
	return nil
}

func (p *wasmPlugin) isGRPCClient(name string) bool {
	for _, cli := range p.exportedValue.GRPCClients {
		if cli == name {
			return true
		}
	}
	return false
}

// ExtractByKey implements query.KeyExtractor interface.
func (p *wasmPlugin) ExtractByKey(key string) (any, bool) {
	fmt.Println("key", key)
	if p.isGRPCClient(key) {
		return &wasmGRPCClient{
			plugin:     p,
			clientName: key,
		}, true
	}
	return nil, false
}

type wasmGRPCClient struct {
	plugin     *wasmPlugin
	clientName string
}

type ExistsMethodRequest struct {
	ClientName string `json:"clientName"`
	MethodName string `json:"methodName"`
}

type ExistsMethodResponse struct {
	Exists bool   `json:"exists"`
	Error  string `json:"error"`
}

func (c *wasmGRPCClient) ExistsMethod(method string) bool {
	fmt.Println("exists method", method)
	req, err := json.Marshal(&ExistsMethodRequest{
		ClientName: c.clientName,
		MethodName: method,
	})
	if err != nil {
		return false
	}
	reqAddr, err := c.plugin.mod.ExportedFunction("__alloc").Call(c.plugin.ctx, uint64(len(req)))
	if err != nil {
		return false
	}
	if ok := c.plugin.mod.Memory().Write(uint32(reqAddr[0]), req); !ok {
		return false
	}
	bytes, err := callExportedFunction(
		c.plugin.ctx,
		c.plugin.mod,
		"__scenarigo_plugin_grpc_client_exists_method",
		[]uint64{
			uint64(reqAddr[0]),
			uint64(len(req)),
		},
	)
	if err != nil {
		return false
	}
	var v ExistsMethodResponse
	if err := json.Unmarshal(bytes, &v); err != nil {
		return false
	}
	fmt.Println("v", v)
	return v.Exists
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

func (c *wasmGRPCClient) BuildRequestMessage(method string, msg []byte) (proto.Message, error) {
	fmt.Println("build request message", method, string(msg))
	req, err := json.Marshal(&BuildRequest{
		ClientName: c.clientName,
		MethodName: method,
		Message:    msg,
	})
	if err != nil {
		return nil, err
	}
	reqAddr, err := c.plugin.mod.ExportedFunction("__alloc").Call(c.plugin.ctx, uint64(len(req)))
	if err != nil {
		return nil, err
	}
	if ok := c.plugin.mod.Memory().Write(uint32(reqAddr[0]), req); !ok {
		return nil, errors.New("failed to write memory")
	}
	resBytes, err := callExportedFunction(
		c.plugin.ctx,
		c.plugin.mod,
		"__scenarigo_plugin_grpc_client_build_request",
		[]uint64{
			uint64(reqAddr[0]),
			uint64(len(req)),
		},
	)
	if err != nil {
		return nil, err
	}
	var v BuildResponse
	if err := json.Unmarshal(resBytes, &v); err != nil {
		return nil, err
	}
	var fdset descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(v.FDSet, &fdset); err != nil {
		return nil, err
	}
	files, err := protodesc.NewFiles(&fdset)
	if err != nil {
		return nil, err
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(v.MessageFQDN))
	if err != nil {
		return nil, err
	}
	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("failed to convert message descriptor from %T", desc)
	}
	protoMsg := dynamicpb.NewMessage(msgDesc)
	if !bytes.Equal(msg, []byte("null\n")) {
		if err := protojson.Unmarshal(msg, protoMsg); err != nil {
			return nil, err
		}
	}
	fmt.Println("v", protoMsg)
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

func (c *wasmGRPCClient) Invoke(method string, reqProto proto.Message) (proto.Message, *status.Status, error) {
	fmt.Println("invoke", method, reqProto)
	reqMsg, err := protojson.Marshal(reqProto)
	if err != nil {
		return nil, nil, err
	}
	req, err := json.Marshal(&InvokeRequest{
		ClientName: c.clientName,
		MethodName: method,
		Request:    reqMsg,
	})
	if err != nil {
		return nil, nil, err
	}
	reqAddr, err := c.plugin.mod.ExportedFunction("__alloc").Call(c.plugin.ctx, uint64(len(req)))
	if err != nil {
		return nil, nil, err
	}
	if ok := c.plugin.mod.Memory().Write(uint32(reqAddr[0]), req); !ok {
		return nil, nil, errors.New("failed to write memory")
	}
	resBytes, err := callExportedFunction(
		c.plugin.ctx,
		c.plugin.mod,
		"__scenarigo_plugin_grpc_client_invoke",
		[]uint64{
			uint64(reqAddr[0]),
			uint64(len(req)),
		},
	)
	if err != nil {
		return nil, nil, err
	}
	var v InvokeResponse
	if err := json.Unmarshal(resBytes, &v); err != nil {
		return nil, nil, err
	}
	var fdset descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(v.FDSet, &fdset); err != nil {
		return nil, nil, err
	}
	files, err := protodesc.NewFiles(&fdset)
	if err != nil {
		return nil, nil, err
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(v.ResponseFQDN))
	if err != nil {
		return nil, nil, err
	}
	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, nil, fmt.Errorf("failed to convert message descriptor from %T", desc)
	}
	protoMsg := dynamicpb.NewMessage(msgDesc)
	if !bytes.Equal(v.ResponseBytes, []byte("null")) {
		if err := protojson.Unmarshal(v.ResponseBytes, protoMsg); err != nil {
			return nil, nil, err
		}
	}
	fmt.Println("responseMsg", protoMsg)
	var stProto stpb.Status
	if err := proto.Unmarshal(v.StatusProto, &stProto); err != nil {
		return nil, nil, err
	}
	st := status.FromProto(&stProto)
	fmt.Println("status", st)
	return protoMsg, st, nil
}

func callExportedFunction(ctx context.Context, mod api.Module, name string, args []uint64) ([]byte, error) {
	retAddr, err := mod.ExportedFunction("__alloc").Call(ctx, 4)
	if err != nil {
		return nil, err
	}
	retLength, err := mod.ExportedFunction("__alloc").Call(ctx, 4)
	if err != nil {
		return nil, err
	}
	if _, err := mod.ExportedFunction(name).Call(ctx, append(args, retAddr[0], retLength[0])...); err != nil {
		return nil, err
	}
	addr, ok := mod.Memory().ReadUint32Le(uint32(retAddr[0]))
	if !ok {
		return nil, fmt.Errorf("failed to read return value address")
	}
	length, ok := mod.Memory().ReadUint32Le(uint32(retLength[0]))
	if !ok {
		return nil, fmt.Errorf("failed to read return value length")
	}
	bytes, ok := mod.Memory().Read(addr, length)
	if !ok {
		return nil, fmt.Errorf(
			`failed to read wasm memory: (ptr, size) = (%d, %d) and memory size is %d`,
			addr, length, mod.Memory().Size(),
		)
	}
	return bytes, nil
}
