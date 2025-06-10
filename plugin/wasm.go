package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/goccy/wasi-go/imports"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var ignoreEnvNameMap = map[string]struct{}{
	// If a value greater than 1 is passed to GOMAXPROCS, a panic occurs on the plugin side,
	// so make sure not to pass it explicitly.
	"GOMAXPROCS": {},
}

func openWasmPlugin(path string) (Plugin, error) {
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

type ExistsMethodRequest struct {
	ClientName string `json:"clientName"`
	MethodName string `json:"methodName"`
}

type ExistsMethodResponse struct {
	Exists bool   `json:"exists"`
	Error  string `json:"error"`
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
	return func(ctx *Context) (*Context, func(*Context)) {
		fmt.Println("called setup by host")
		if _, err := callExportedFunction(
			p.ctx,
			p.mod,
			"__scenarigo_plugin_setup",
			nil,
		); err != nil {
			fmt.Println("err", err)
		}
		return ctx, func(ctx *Context) {
			_, _ = callExportedFunction(
				p.ctx,
				p.mod,
				"__scenarigo_plugin_teardown",
				nil,
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

func (c *wasmGRPCClient) ExistsMethod(method string) bool {
	ctx := context.Background()
	//　この中で wasm 側の関数を呼ぶ。gRPC Client の名前と method 名を渡して判定結果をもらう感じ。
	fmt.Println("exists method", method)
	req, err := json.Marshal(&ExistsMethodRequest{
		ClientName: c.clientName,
		MethodName: method,
	})
	if err != nil {
		return false
	}
	reqAddr, err := c.plugin.mod.ExportedFunction("__alloc").Call(ctx, uint64(len(req)))
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

func (c *wasmGRPCClient) BuildRequestMessage(method string, params any) (proto.Message, error) {
	fmt.Println("build request message", method, params)
	//　この中で wasm 側の関数を呼ぶ。 newCustomServiceClient の中でやっている処理を wasm 側で実行して結果をもらう
	return nil, nil
}

func (c *wasmGRPCClient) Invoke(method string, req proto.Message) (proto.Message, *status.Status, error) {
	fmt.Println("invoke", method, req)
	return nil, nil, nil
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
	fmt.Println("mod", mod, "args", args)
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
