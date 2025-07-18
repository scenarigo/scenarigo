package grpc

import (
	"bytes"
	gocontext "context"
	"fmt"
	"reflect"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/plugin"
)

type customServiceClient struct {
	r            *Request
	method       reflect.Value
	customClient plugin.CustomGRPCClient
}

func newCustomServiceClient(r *Request, x any) (*customServiceClient, error) {
	if cli, ok := x.(plugin.CustomGRPCClient); ok {
		if !cli.ExistsMethod(r.Method) {
			return nil, errors.ErrorPathf("method", `method "%s.%s" not found`, r.Client, r.Method)
		}
		return &customServiceClient{
			r:            r,
			customClient: cli,
		}, nil
	}

	v := reflect.ValueOf(x)

	var method reflect.Value
	for {
		if !v.IsValid() {
			return nil, errors.ErrorPathf("client", "client %q is invalid", r.Client)
		}
		method = v.MethodByName(r.Method)
		if method.IsValid() {
			// method found
			break
		}
		switch v.Kind() {
		case reflect.Interface, reflect.Ptr:
			v = v.Elem()
		default:
			return nil, errors.ErrorPathf("method", `method "%s.%s" not found`, r.Client, r.Method)
		}
	}

	if err := plugin.ValidateGRPCMethod(method); err != nil {
		return nil, errors.ErrorPathf("method", `"%s.%s" must be "func(context.Context, proto.Message, ...grpc.CallOption) (proto.Message, error): %s"`, r.Client, r.Method, err)
	}

	return &customServiceClient{
		r:      r,
		method: method,
	}, nil
}

func (client *customServiceClient) buildRequestMessage(ctx *context.Context) (proto.Message, error) {
	if client.customClient != nil {
		msg, err := ctx.ExecuteTemplate(client.r.Message)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err := yaml.NewEncoder(&buf, yaml.JSON()).Encode(msg); err != nil {
			return nil, err
		}
		return client.customClient.BuildRequestMessage(client.r.Method, bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
	}

	req := reflect.New(client.method.Type().In(1).Elem()).Interface()
	if err := buildRequestMsg(ctx, req, client.r.Message); err != nil {
		return nil, errors.WrapPathf(err, "message", "failed to build request message")
	}
	reqMsg, ok := req.(proto.Message)
	if !ok {
		return nil, errors.ErrorPathf("client", "failed to build request message: second argument must be proto.Message but %T", req)
	}
	return reqMsg, nil
}

func (client *customServiceClient) invoke(ctx gocontext.Context, reqMsg proto.Message, opts ...grpc.CallOption) (proto.Message, *status.Status, error) {
	if client.customClient != nil {
		return client.customClient.Invoke(ctx, client.r.Method, reqMsg)
	}
	return plugin.GRPCInvoke(ctx, client.method, reqMsg, opts...)
}

func buildRequestMsg(ctx *context.Context, req any, src any) error {
	x, err := ctx.ExecuteTemplate(src)
	if err != nil {
		return err
	}
	if x == nil {
		return nil
	}
	msg, ok := req.(proto.Message)
	if !ok {
		return fmt.Errorf("expect proto.Message but got %T", req)
	}
	return ConvertToProto(x, msg)
}

func ConvertToProto(v any, msg proto.Message) error {
	var buf bytes.Buffer
	if err := yaml.NewEncoder(&buf, yaml.JSON()).Encode(v); err != nil {
		return err
	}
	if err := protojson.Unmarshal(buf.Bytes(), msg); err != nil {
		return err
	}
	return nil
}
