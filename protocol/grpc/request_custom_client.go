package grpc

import (
	"bytes"
	gocontext "context"
	stderrors "errors"
	"fmt"
	"io"
	"reflect"
	"sync"

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
	}
	return msgs, nil
}

func (client *customServiceClient) invoke(ctx gocontext.Context, reqMsg proto.Message, opts ...grpc.CallOption) (proto.Message, *status.Status, error) {
	if client.customClient != nil {
		return client.customClient.Invoke(ctx, client.r.Method, reqMsg)
	}
	return plugin.GRPCInvoke(ctx, client.method, reqMsg, opts...)
}

func (client *customServiceClient) invokeServerStream(ctx gocontext.Context, reqMsg proto.Message, opts ...grpc.CallOption) ([]proto.Message, metadata.MD, metadata.MD, *status.Status, error) {
	stream, err := plugin.GRPCInvokeServerStream(ctx, client.method, reqMsg, opts...)
	if err != nil {
		return nil, nil, nil, status.Convert(err), nil
	}

	var msgs []proto.Message
	for {
		msg, err := plugin.GRPCStreamRecv(stream)
		if err != nil {
			if stderrors.Is(err, io.EOF) {
				break
			}
			cs, _ := plugin.GRPCStreamAsClientStream(stream)
			var header metadata.MD
			if cs != nil {
				header, _ = cs.Header()
			}
			var trailer metadata.MD
			if cs != nil {
				trailer = cs.Trailer()
			}
			return nil, header, trailer, status.Convert(err), nil
		}
		msgs = append(msgs, msg)
	}

	cs, _ := plugin.GRPCStreamAsClientStream(stream)
	var header metadata.MD
	var trailer metadata.MD
	if cs != nil {
		header, _ = cs.Header()
		trailer = cs.Trailer()
	}
	return msgs, header, trailer, nil, nil
}

func (client *customServiceClient) invokeClientStream(ctx gocontext.Context, msgs []proto.Message, opts ...grpc.CallOption) (proto.Message, metadata.MD, metadata.MD, *status.Status, error) {
	stream, err := plugin.GRPCInvokeStream(ctx, client.method, opts...)
	if err != nil {
		return nil, nil, nil, status.Convert(err), nil
	}

	for _, msg := range msgs {
		if err := plugin.GRPCStreamSend(stream, msg); err != nil {
			return nil, nil, nil, status.Convert(err), nil
		}
	}

	respMsg, err := plugin.GRPCStreamCloseAndRecv(stream)
	if err != nil {
		cs, _ := plugin.GRPCStreamAsClientStream(stream)
		var header metadata.MD
		var trailer metadata.MD
		if cs != nil {
			header, _ = cs.Header()
			trailer = cs.Trailer()
		}
		return nil, header, trailer, status.Convert(err), nil
	}

	cs, _ := plugin.GRPCStreamAsClientStream(stream)
	var header metadata.MD
	var trailer metadata.MD
	if cs != nil {
		header, _ = cs.Header()
		trailer = cs.Trailer()
	}
	return respMsg, header, trailer, nil, nil
}

func (client *customServiceClient) invokeBidiStream(ctx gocontext.Context, sCtx *context.Context, opts ...grpc.CallOption) ([]proto.Message, metadata.MD, metadata.MD, *status.Status, error) {
	stream, err := plugin.GRPCInvokeStream(ctx, client.method, opts...)
	if err != nil {
		return nil, nil, nil, status.Convert(err), nil
	}

	reqType, err := plugin.GRPCStreamRequestType(client.method, client.methodType)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to determine request type")
	}

	// Start background receiver goroutine
	type recvResult struct {
		msgs []proto.Message
		err  error
	}
	recvCh := make(chan recvResult, 1)
	var (
		mu       sync.Mutex
		received []proto.Message
		recvDone bool
		cond     = sync.NewCond(&mu)
	)
	go func() {
		var msgs []proto.Message
		for {
			msg, err := plugin.GRPCStreamRecv(stream)
			if err != nil {
				if stderrors.Is(err, io.EOF) {
					mu.Lock()
					recvDone = true
					cond.Broadcast()
					mu.Unlock()
					recvCh <- recvResult{msgs: msgs}
					return
				}
				mu.Lock()
				recvDone = true
				cond.Broadcast()
				mu.Unlock()
				recvCh <- recvResult{err: err}
				return
			}
			mu.Lock()
			msgs = append(msgs, msg)
			received = msgs
			cond.Broadcast()
			mu.Unlock()
		}
	}()

	// Set up a response accessor that blocks until the Nth response is available
	bidiResp := &bidiResponseAccessor{
		mu:       &mu,
		cond:     cond,
		received: &received,
		recvDone: &recvDone,
	}
	sCtx = sCtx.WithResponse(bidiResp)

	// Set up a request accessor for referencing already-sent messages
	bidiReq := &bidiRequestAccessor{}
	sCtx = sCtx.WithRequest(bidiReq)

	// Send messages sequentially, evaluating templates as we go
	for i, m := range client.r.Messages {
		x, err := sCtx.ExecuteTemplate(m)
		if err != nil {
			return nil, nil, nil, nil, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to execute template")
		}
		in, ok := reflect.New(reqType).Interface().(proto.Message)
		if !ok {
			return nil, nil, nil, nil, errors.ErrorPathf(fmt.Sprintf("messages[%d]", i), "expected proto.Message but got %T", reflect.New(reqType).Interface())
		}
		if x != nil {
			if err := ConvertToProto(x, in); err != nil {
				return nil, nil, nil, nil, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to build request message")
			}
		}
		if err := plugin.GRPCStreamSend(stream, in); err != nil {
			return nil, nil, nil, status.Convert(err), nil
		}
		bidiReq.sent = append(bidiReq.sent, in)
	}

	if err := plugin.GRPCStreamCloseSend(stream); err != nil {
		return nil, nil, nil, status.Convert(err), nil
	}

	// Wait for all responses
	result := <-recvCh
	if recvErr := result.err; recvErr != nil {
		cs, _ := plugin.GRPCStreamAsClientStream(stream)
		var header metadata.MD
		var trailer metadata.MD
		if cs != nil {
			header, _ = cs.Header()
			trailer = cs.Trailer()
		}
		return nil, header, trailer, status.Convert(recvErr), nil
	}

	cs, _ := plugin.GRPCStreamAsClientStream(stream)
	var header metadata.MD
	var trailer metadata.MD
	if cs != nil {
		header, _ = cs.Header()
		trailer = cs.Trailer()
	}
	return result.msgs, header, trailer, nil, nil
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
