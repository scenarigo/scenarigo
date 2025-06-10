package grpc

import (
	gocontext "context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
)

type wasmPluginCustomServiceClient struct {
	r    *Request
	wasm WasmPluginGRPCClient
}

func newWasmPluginCustomServiceClient(r *Request, v WasmPluginGRPCClient) (*wasmPluginCustomServiceClient, error) {
	if !v.ExistsMethod(r.Method) {
		return nil, errors.ErrorPathf("method", `method "%s.%s" not found`, r.Client, r.Method)
	}
	return &wasmPluginCustomServiceClient{
		r:    r,
		wasm: v,
	}, nil
}

func (client *wasmPluginCustomServiceClient) buildRequestMessage(ctx *context.Context) (proto.Message, error) {
	req, err := ctx.ExecuteTemplate(client.r.Message)
	if err != nil {
		return nil, err
	}
	return client.wasm.BuildRequestMessage(client.r.Method, req)
}

func (client *wasmPluginCustomServiceClient) invoke(ctx gocontext.Context, reqMsg proto.Message, opts ...grpc.CallOption) (proto.Message, *status.Status, error) {
	return client.wasm.Invoke(client.r.Method, reqMsg)
}
