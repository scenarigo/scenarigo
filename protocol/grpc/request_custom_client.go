package grpc

import (
	"bytes"
	gocontext "context"
	stderrors "errors"
	"fmt"
	"io"
	"reflect"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/grpcstream"
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

func (client *customServiceClient) invokeServerStream(ctx gocontext.Context, reqMsg proto.Message, opts ...grpc.CallOption) (*streamResult, error) {
	stream, err := plugin.GRPCInvokeServerStream(ctx, client.method, reqMsg, opts...)
	if err != nil {
		return &streamResult{sts: status.Convert(err)}, nil
	}

	var msgs []proto.Message
	for {
		msg, err := plugin.GRPCStreamRecv(stream)
		if err != nil {
			if stderrors.Is(err, io.EOF) {
				break
			}
			// Keep the messages received so far to dump them for debugging.
			header, trailer := streamHeaderTrailer(stream)
			return &streamResult{messages: msgs, header: header, trailer: trailer, sts: status.Convert(err)}, nil
		}
		msgs = append(msgs, msg)
	}

	header, trailer := streamHeaderTrailer(stream)
	return &streamResult{messages: msgs, header: header, trailer: trailer}, nil
}

func (client *customServiceClient) invokeClientStream(ctx gocontext.Context, msgs []proto.Message, opts ...grpc.CallOption) (*streamResult, error) {
	stream, err := plugin.GRPCInvokeStream(ctx, client.method, opts...)
	if err != nil {
		return &streamResult{sts: status.Convert(err)}, nil
	}

	for _, msg := range msgs {
		if err := plugin.GRPCStreamSend(stream, msg); err != nil {
			// Send returns io.EOF when the server terminates the stream;
			// the actual status is retrieved by CloseAndRecv below.
			if stderrors.Is(err, io.EOF) {
				break
			}
			return &streamResult{sts: status.Convert(err)}, nil
		}
	}

	respMsg, err := plugin.GRPCStreamCloseAndRecv(stream)
	if err != nil {
		header, trailer := streamHeaderTrailer(stream)
		return &streamResult{header: header, trailer: trailer, sts: status.Convert(err)}, nil
	}

	header, trailer := streamHeaderTrailer(stream)
	return &streamResult{message: respMsg, header: header, trailer: trailer}, nil
}

func (client *customServiceClient) invokeBidiStream(ctx gocontext.Context, sCtx *context.Context, opts ...grpc.CallOption) (*streamResult, error) {
	result := &streamResult{}
	// Cancel the stream when we return so the background receiver goroutine and
	// the server RPC are released even if we bail out mid-stream (e.g. when the
	// deadlock guard fires while evaluating a response reference).
	streamCtx, cancelStream := gocontext.WithCancel(ctx)
	defer cancelStream()
	stream, err := plugin.GRPCInvokeStream(streamCtx, client.method, opts...)
	if err != nil {
		result.sts = status.Convert(err)
		return result, nil
	}

	reqType, err := plugin.GRPCStreamRequestType(client.method, client.methodType)
	if err != nil {
		return result, errors.Wrap(err, "failed to determine request type")
	}

	// Accumulate responses in the background. Blocking response references
	// (response.messages[N]) wait on the buffer, bounded by the template
	// evaluation context, so a deadlocked scenario fails instead of hanging.
	buf := grpcstream.NewBuffer[proto.Message]()
	recvCh := make(chan error, 1)
	go func() {
		for {
			msg, err := plugin.GRPCStreamRecv(stream)
			if err != nil {
				buf.Close()
				if stderrors.Is(err, io.EOF) {
					err = nil
				}
				recvCh <- err
				return
			}
			buf.Append(msg)
		}
	}()

	// Set up a response accessor that blocks until the Nth response is available
	bidiResp := &bidiResponseAccessor{buf: buf}
	sCtx = sCtx.WithResponse(bidiResp)

	// Set up a request accessor for referencing already-sent messages
	bidiReq := &requestMessagesAccessor{}
	sCtx = sCtx.WithRequest(bidiReq)

	// Send messages sequentially, evaluating templates as we go
	for i, m := range client.r.Messages {
		x, err := sCtx.ExecuteTemplate(m)
		if err != nil {
			result.messages = buf.Snapshot()
			if buf.WaitErr() != nil {
				return result, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "interrupted while waiting for a streaming response message (possible deadlock or timeout)")
			}
			return result, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to execute template")
		}
		in, ok := reflect.New(reqType).Interface().(proto.Message)
		if !ok {
			result.messages = buf.Snapshot()
			return result, errors.ErrorPathf(fmt.Sprintf("messages[%d]", i), "expected proto.Message but got %T", reflect.New(reqType).Interface())
		}
		if x != nil {
			if err := ConvertToProto(x, in); err != nil {
				result.messages = buf.Snapshot()
				return result, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to build request message")
			}
		}
		// Record the message before sending so that failed attempts also appear in the dump.
		bidiReq.sent = append(bidiReq.sent, in)
		result.sent = bidiReq.sent
		if err := plugin.GRPCStreamSend(stream, in); err != nil {
			// Send returns io.EOF when the server terminates the stream;
			// the actual status is reported by the receiver goroutine.
			if stderrors.Is(err, io.EOF) {
				break
			}
			result.messages = buf.Snapshot()
			result.sts = status.Convert(err)
			return result, nil
		}
	}

	if err := plugin.GRPCStreamCloseSend(stream); err != nil {
		result.messages = buf.Snapshot()
		result.sts = status.Convert(err)
		return result, nil
	}

	// Wait for all responses
	recvErr := <-recvCh
	result.messages = buf.Snapshot()
	result.header, result.trailer = streamHeaderTrailer(stream)
	if recvErr != nil {
		result.sts = status.Convert(recvErr)
	}
	return result, nil
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
