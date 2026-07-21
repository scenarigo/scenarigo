package grpc

import (
	"bytes"
	gocontext "context"
	"fmt"
	"reflect"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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
	methodType   plugin.GRPCMethodType
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

	methodType := plugin.DetectGRPCMethodType(method)

	switch methodType {
	case plugin.GRPCMethodUnary:
		if err := plugin.ValidateGRPCMethod(method); err != nil {
			return nil, errors.ErrorPathf("method", `"%s.%s" must be "func(context.Context, proto.Message, ...grpc.CallOption) (proto.Message, error): %s"`, r.Client, r.Method, err)
		}
	default:
		if err := plugin.ValidateGRPCStreamingMethod(method, methodType); err != nil {
			return nil, errors.ErrorPathf("method", `"%s.%s" is not a valid streaming method: %s`, r.Client, r.Method, err)
		}
	}

	return &customServiceClient{
		r:          r,
		method:     method,
		methodType: methodType,
	}, nil
}

func (client *customServiceClient) isStreamingClient() bool {
	return client.methodType == plugin.GRPCMethodClientStream || client.methodType == plugin.GRPCMethodBidiStream
}

func (client *customServiceClient) isStreamingServer() bool {
	return client.methodType == plugin.GRPCMethodServerStream || client.methodType == plugin.GRPCMethodBidiStream
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

	reqType, err := plugin.GRPCStreamRequestType(client.method, client.methodType)
	if err != nil {
		return nil, errors.WrapPathf(err, "message", "failed to determine request type")
	}
	req := reflect.New(reqType).Interface()
	if err := buildRequestMsg(ctx, req, client.r.Message); err != nil {
		return nil, errors.WrapPathf(err, "message", "failed to build request message")
	}
	reqMsg, ok := req.(proto.Message)
	if !ok {
		return nil, errors.ErrorPathf("client", "failed to build request message: second argument must be proto.Message but %T", req)
	}
	return reqMsg, nil
}

func (client *customServiceClient) buildRequestMessages(ctx *context.Context) ([]proto.Message, error) {
	reqType, err := plugin.GRPCStreamRequestType(client.method, client.methodType)
	if err != nil {
		return nil, errors.WrapPathf(err, "messages", "failed to determine request type")
	}
	// Allow each message template to reference the already-built messages via request.messages[N].
	reqAccessor := &requestMessagesAccessor{}
	ctx = ctx.WithRequest(reqAccessor)
	msgs := make([]proto.Message, len(client.r.Messages))
	for i, m := range client.r.Messages {
		req := reflect.New(reqType).Interface()
		if err := buildRequestMsg(ctx, req, m); err != nil {
			return nil, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to build request message")
		}
		msg, ok := req.(proto.Message)
		if !ok {
			return nil, errors.ErrorPathf(fmt.Sprintf("messages[%d]", i), "expected proto.Message but got %T", req)
		}
		msgs[i] = msg
		reqAccessor.sent = append(reqAccessor.sent, msg)
	}
	return msgs, nil
}

func (client *customServiceClient) invoke(ctx gocontext.Context, reqMsg proto.Message, opts ...grpc.CallOption) (proto.Message, *status.Status, error) {
	if client.customClient != nil {
		return client.customClient.Invoke(ctx, client.r.Method, reqMsg, opts...)
	}
	return plugin.GRPCInvoke(ctx, client.method, reqMsg, opts...)
}

// streamHeaderTrailer extracts the header and trailer metadata from a stream.
func streamHeaderTrailer(stream reflect.Value) (metadata.MD, metadata.MD) {
	cs, _ := plugin.GRPCStreamAsClientStream(stream)
	if cs == nil {
		return nil, nil
	}
	header, _ := cs.Header()
	return header, cs.Trailer()
}

// customStreamConn adapts a reflection-driven stream from a generated client
// to the per-kind stream connection interfaces.
type customStreamConn struct {
	stream  reflect.Value
	reqType reflect.Type
}

func (c *customStreamConn) NewInput() (proto.Message, error) {
	in, ok := reflect.New(c.reqType).Interface().(proto.Message)
	if !ok {
		return nil, fmt.Errorf("expected proto.Message but got %T", reflect.New(c.reqType).Interface())
	}
	return in, nil
}

func (c *customStreamConn) Send(msg proto.Message) error {
	return plugin.GRPCStreamSend(c.stream, msg)
}

func (c *customStreamConn) Recv() (proto.Message, error) {
	return plugin.GRPCStreamRecv(c.stream)
}

func (c *customStreamConn) CloseSend() error {
	return plugin.GRPCStreamCloseSend(c.stream)
}

func (c *customStreamConn) CloseAndRecv() (proto.Message, error) {
	return plugin.GRPCStreamCloseAndRecv(c.stream)
}

func (c *customStreamConn) HeaderTrailer() (metadata.MD, metadata.MD) {
	return streamHeaderTrailer(c.stream)
}

func (client *customServiceClient) invokeServerStream(ctx gocontext.Context, reqMsg proto.Message, opts ...grpc.CallOption) (*streamResult, error) {
	return runServerStream(func() (serverStreamConn, error) {
		stream, err := plugin.GRPCInvokeServerStream(ctx, client.method, reqMsg, opts...)
		if err != nil {
			return nil, err
		}
		return &customStreamConn{stream: stream}, nil
	})
}

func (client *customServiceClient) invokeClientStream(ctx gocontext.Context, msgs []proto.Message, opts ...grpc.CallOption) (*streamResult, error) {
	return runClientStream(func() (clientStreamConn, error) {
		stream, err := plugin.GRPCInvokeStream(ctx, client.method, opts...)
		if err != nil {
			return nil, err
		}
		return &customStreamConn{stream: stream}, nil
	}, msgs)
}

func (client *customServiceClient) invokeBidiStream(ctx gocontext.Context, sCtx *context.Context, opts ...grpc.CallOption) (*streamResult, error) {
	reqType, err := plugin.GRPCStreamRequestType(client.method, client.methodType)
	if err != nil {
		return &streamResult{}, errors.Wrap(err, "failed to determine request type")
	}
	return runBidiStream(ctx, sCtx, client.r.Messages, func(streamCtx gocontext.Context) (bidiStreamConn, error) {
		stream, err := plugin.GRPCInvokeStream(streamCtx, client.method, opts...)
		if err != nil {
			return nil, err
		}
		return &customStreamConn{stream: stream, reqType: reqType}, nil
	})
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
