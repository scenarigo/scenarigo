package grpc

import (
	gocontext "context"
	stderrors "errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/goccy/go-yaml"

	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/assertutil"
	"github.com/scenarigo/scenarigo/internal/yamlutil"
	grpcprotocol "github.com/scenarigo/scenarigo/protocol/grpc"
)

const messagesKey = "messages"

func (s *server) convertToServicDesc(sd protoreflect.ServiceDescriptor) *grpc.ServiceDesc {
	desc := &grpc.ServiceDesc{
		ServiceName: string(sd.FullName()),
		Metadata:    sd.ParentFile().Path(),
	}
	for i := range sd.Methods().Len() {
		m := sd.Methods().Get(i)
		if m.IsStreamingServer() || m.IsStreamingClient() {
			desc.Streams = append(desc.Streams, grpc.StreamDesc{
				StreamName:    string(m.Name()),
				ServerStreams: m.IsStreamingServer(),
				ClientStreams: m.IsStreamingClient(),
				Handler:       s.streamHandler(sd.FullName(), m),
			})
		} else {
			desc.Methods = append(desc.Methods, grpc.MethodDesc{
				MethodName: string(m.Name()),
				Handler:    s.unaryHandler(sd.FullName(), m),
			})
		}
	}
	return desc
}

func (s *server) unaryHandler(svcName protoreflect.FullName, method protoreflect.MethodDescriptor) func(srv any, ctx gocontext.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx gocontext.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		mock, err := s.iter.Next()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get mock: %s", err)
		}

		if mock.Protocol != protocolName {
			return nil, status.Error(codes.Internal, errors.WithPath(fmt.Errorf("received gRPC request but the mock protocol is %q", mock.Protocol), "protocol").Error())
		}

		var e expect
		if err := mock.Expect.Unmarshal(&e); err != nil {
			return nil, status.Error(codes.Internal, errors.WrapPath(err, "expect", "failed to unmarshal").Error())
		}
		assertion, err := e.build(context.New(nil))
		if err != nil {
			return nil, status.Error(codes.Internal, errors.WrapPath(err, "expect", "failed to build assretion").Error())
		}

		var md metadata.MD
		if got, ok := metadata.FromIncomingContext(ctx); ok {
			md = got
		}
		req := dynamicpb.NewMessage(method.Input())
		if err := dec(req); err != nil {
			return nil, status.Error(codes.Internal, errors.WrapPath(err, "expect.message", "failed to decode message").Error())
		}
		if err := assertion.Assert(&request{
			service:  string(svcName),
			method:   string(method.Name()),
			metadata: yamlutil.NewMDMarshaler(md),
			message:  req,
		}); err != nil {
			return nil, status.Error(codes.InvalidArgument, errors.WrapPath(err, "expect", "request assertion failed").Error())
		}

		var resp Response
		if err := mock.Response.Unmarshal(&resp); err != nil {
			return nil, status.Error(codes.Internal, errors.WrapPath(err, "response", "failed to unmarshal response").Error())
		}
		sctx := context.New(nil)
		v, err := sctx.ExecuteTemplate(resp)
		if err != nil {
			return nil, status.Error(codes.Internal, errors.WrapPath(err, "response", "failed to execute template of response").Error())
		}
		resp, ok := v.(Response)
		if !ok {
			return nil, status.Error(codes.Internal, errors.WithPath(fmt.Errorf("failed to execute template of response: unexpected type %T", v), "response").Error())
		}

		var msg proto.Message = dynamicpb.NewMessage(method.Output())
		msg, serr, err := resp.extract(msg)
		if err != nil {
			return nil, status.Error(codes.Internal, errors.WithPath(err, "response").Error())
		}
		return msg, serr.Err()
	}
}

func (s *server) streamHandler(svcName protoreflect.FullName, method protoreflect.MethodDescriptor) grpc.StreamHandler {
	return func(srv any, stream grpc.ServerStream) error {
		mock, err := s.iter.Next()
		if err != nil {
			return status.Errorf(codes.Internal, "failed to get mock: %s", err)
		}

		if mock.Protocol != protocolName {
			return status.Error(codes.Internal, errors.WithPath(fmt.Errorf("received gRPC request but the mock protocol is %q", mock.Protocol), "protocol").Error())
		}

		var e expect
		if err := mock.Expect.Unmarshal(&e); err != nil {
			return status.Error(codes.Internal, errors.WrapPath(err, "expect", "failed to unmarshal").Error())
		}

		var resp Response
		if err := mock.Response.Unmarshal(&resp); err != nil {
			return status.Error(codes.Internal, errors.WrapPath(err, "response", "failed to unmarshal response").Error())
		}

		switch {
		case method.IsStreamingClient() && method.IsStreamingServer():
			return s.handleBidiStream(stream, method, svcName, &e, &resp)
		case method.IsStreamingServer():
			return s.handleServerStream(stream, method, svcName, &e, &resp)
		default: // client streaming
			return s.handleClientStream(stream, method, svcName, &e, &resp)
		}
	}
}

func (s *server) handleServerStream(stream grpc.ServerStream, method protoreflect.MethodDescriptor, svcName protoreflect.FullName, e *expect, resp *Response) error {
	// Receive single request message
	req := dynamicpb.NewMessage(method.Input())
	if err := stream.RecvMsg(req); err != nil {
		return status.Error(codes.Internal, errors.WrapPath(err, "expect.message", "failed to receive message").Error())
	}

	// Build and run assertion
	assertion, err := e.build(context.New(nil))
	if err != nil {
		return status.Error(codes.Internal, errors.WrapPath(err, "expect", "failed to build assertion").Error())
	}
	var md metadata.MD
	if got, ok := metadata.FromIncomingContext(stream.Context()); ok {
		md = got
	}
	if err := assertion.Assert(&request{
		service:  string(svcName),
		method:   string(method.Name()),
		metadata: yamlutil.NewMDMarshaler(md),
		message:  req,
	}); err != nil {
		return status.Error(codes.InvalidArgument, errors.WrapPath(err, "expect", "request assertion failed").Error())
	}

	// Check status
	if resp.Status.Code != "" {
		code, err := strToCode(resp.Status.Code)
		if err != nil {
			return status.Error(codes.Internal, errors.WithPath(err, "response.status.code").Error())
		}
		if code != codes.OK {
			smsg := code.String()
			if resp.Status.Message != "" {
				smsg = resp.Status.Message
			}
			return status.Error(code, smsg)
		}
	}

	// Set up template context with request.message
	sctx := context.New(nil)
	sctx = sctx.WithRequest(req)

	// Send multiple response messages
	msgs, err := resp.extractMessages(sctx, method)
	if err != nil {
		return status.Error(codes.Internal, errors.WithPath(err, "response").Error())
	}
	for _, msg := range msgs {
		if err := stream.SendMsg(msg); err != nil {
			return status.Error(codes.Internal, errors.WrapPath(err, "response", "failed to send message").Error())
		}
	}
	return nil
}

func (s *server) handleClientStream(stream grpc.ServerStream, method protoreflect.MethodDescriptor, svcName protoreflect.FullName, e *expect, resp *Response) error {
	// Receive all request messages until EOF
	var received []proto.Message
	for {
		req := dynamicpb.NewMessage(method.Input())
		if err := stream.RecvMsg(req); err != nil {
			if stderrors.Is(err, io.EOF) {
				break
			}
			return status.Error(codes.Internal, errors.WrapPath(err, "expect.messages", "failed to receive message").Error())
		}
		received = append(received, req)
	}

	// Build and run assertion
	assertion, err := e.build(context.New(nil))
	if err != nil {
		return status.Error(codes.Internal, errors.WrapPath(err, "expect", "failed to build assertion").Error())
	}
	var md metadata.MD
	if got, ok := metadata.FromIncomingContext(stream.Context()); ok {
		md = got
	}

	// Wrap received messages for assertion
	msgs := make([]*grpcprotocol.ProtoMessageYAMLMarshaler, len(received))
	for i, m := range received {
		msgs[i] = &grpcprotocol.ProtoMessageYAMLMarshaler{Message: m}
	}

	if err := assertion.Assert(&request{
		service:  string(svcName),
		method:   string(method.Name()),
		metadata: yamlutil.NewMDMarshaler(md),
		messages: msgs,
	}); err != nil {
		return status.Error(codes.InvalidArgument, errors.WrapPath(err, "expect", "request assertion failed").Error())
	}

	// Check status
	if resp.Status.Code != "" {
		code, err := strToCode(resp.Status.Code)
		if err != nil {
			return status.Error(codes.Internal, errors.WithPath(err, "response.status.code").Error())
		}
		if code != codes.OK {
			smsg := code.String()
			if resp.Status.Message != "" {
				smsg = resp.Status.Message
			}
			return status.Error(code, smsg)
		}
	}

	// Set up template context with request.messages
	sctx := context.New(nil)
	sctx = sctx.WithRequest(&clientStreamRequestAccessor{received: msgs})

	// Execute template and extract single response message
	v, err := sctx.ExecuteTemplate(*resp)
	if err != nil {
		return status.Error(codes.Internal, errors.WrapPath(err, "response", "failed to execute template of response").Error())
	}
	executed, ok := v.(Response)
	if !ok {
		return status.Error(codes.Internal, errors.WithPath(fmt.Errorf("failed to execute template of response: unexpected type %T", v), "response").Error())
	}

	msg := dynamicpb.NewMessage(method.Output())
	msg2, _, err := executed.extract(msg)
	if err != nil {
		return status.Error(codes.Internal, errors.WithPath(err, "response").Error())
	}
	return stream.SendMsg(msg2)
}

func (s *server) handleBidiStream(stream grpc.ServerStream, method protoreflect.MethodDescriptor, svcName protoreflect.FullName, e *expect, resp *Response) error {
	// Start background receiver goroutine
	type recvResult struct {
		msgs []*grpcprotocol.ProtoMessageYAMLMarshaler
		err  error
	}
	recvCh := make(chan recvResult, 1)
	var (
		mu       sync.Mutex
		received []*grpcprotocol.ProtoMessageYAMLMarshaler
		recvDone bool
		cond     = sync.NewCond(&mu)
	)
	go func() {
		var msgs []*grpcprotocol.ProtoMessageYAMLMarshaler
		for {
			req := dynamicpb.NewMessage(method.Input())
			if err := stream.RecvMsg(req); err != nil {
				mu.Lock()
				recvDone = true
				cond.Broadcast()
				mu.Unlock()
				if stderrors.Is(err, io.EOF) {
					recvCh <- recvResult{msgs: msgs}
				} else {
					recvCh <- recvResult{err: err}
				}
				return
			}
			m := &grpcprotocol.ProtoMessageYAMLMarshaler{Message: req}
			mu.Lock()
			msgs = append(msgs, m)
			received = msgs
			cond.Broadcast()
			mu.Unlock()
		}
	}()

	// Set up blocking request accessor for template evaluation
	bidiReq := &mockBidiRequestAccessor{
		mu:       &mu,
		cond:     cond,
		received: &received,
		recvDone: &recvDone,
	}

	// Set up response accessor for referencing already-sent responses
	bidiResp := &mockBidiResponseAccessor{}

	sctx := context.New(nil)
	sctx = sctx.WithRequest(bidiReq)
	sctx = sctx.WithResponse(bidiResp)

	// Check status
	if resp.Status.Code != "" {
		code, err := strToCode(resp.Status.Code)
		if err != nil {
			return status.Error(codes.Internal, errors.WithPath(err, "response.status.code").Error())
		}
		if code != codes.OK {
			smsg := code.String()
			if resp.Status.Message != "" {
				smsg = resp.Status.Message
			}
			return status.Error(code, smsg)
		}
	}

	// Send response messages sequentially, evaluating templates as we go
	for i, m := range resp.Messages {
		x, err := sctx.ExecuteTemplate(m)
		if err != nil {
			return status.Error(codes.Internal, errors.WrapPathf(err, fmt.Sprintf("response.messages[%d]", i), "failed to execute template").Error())
		}
		msg := dynamicpb.NewMessage(method.Output())
		if x != nil {
			if err := grpcprotocol.ConvertToProto(x, msg); err != nil {
				return status.Error(codes.Internal, errors.WrapPathf(err, fmt.Sprintf("response.messages[%d]", i), "invalid message").Error())
			}
		}
		if err := stream.SendMsg(msg); err != nil {
			return status.Error(codes.Internal, errors.WrapPath(err, "response", "failed to send message").Error())
		}
		bidiResp.sent = append(bidiResp.sent, &grpcprotocol.ProtoMessageYAMLMarshaler{Message: msg})
	}

	// Wait for all requests to be received
	result := <-recvCh
	if result.err != nil {
		return status.Error(codes.Internal, errors.WrapPath(result.err, "expect", "failed to receive messages").Error())
	}

	// Assert received messages
	assertion, err := e.build(context.New(nil))
	if err != nil {
		return status.Error(codes.Internal, errors.WrapPath(err, "expect", "failed to build assertion").Error())
	}
	var md metadata.MD
	if got, ok := metadata.FromIncomingContext(stream.Context()); ok {
		md = got
	}
	if err := assertion.Assert(&request{
		service:  string(svcName),
		method:   string(method.Name()),
		metadata: yamlutil.NewMDMarshaler(md),
		messages: result.msgs,
	}); err != nil {
		return status.Error(codes.InvalidArgument, errors.WrapPath(err, "expect", "request assertion failed").Error())
	}

	return nil
}

// clientStreamRequestAccessor provides access to received client-stream request messages.
type clientStreamRequestAccessor struct {
	received []*grpcprotocol.ProtoMessageYAMLMarshaler
}

// ExtractByKey implements query.KeyExtractor interface.
func (a *clientStreamRequestAccessor) ExtractByKey(key string) (any, bool) {
	if key == messagesKey {
		return a.received, true
	}
	return nil, false
}

// mockBidiRequestAccessor provides blocking access to request messages received by a background goroutine.
type mockBidiRequestAccessor struct {
	mu       *sync.Mutex
	cond     *sync.Cond
	received *[]*grpcprotocol.ProtoMessageYAMLMarshaler
	recvDone *bool
}

// ExtractByKey implements query.KeyExtractorContext interface.
func (a *mockBidiRequestAccessor) ExtractByKey(_ gocontext.Context, key string) (any, bool) {
	if key == messagesKey {
		return a, true
	}
	return nil, false
}

// ExtractByIndex blocks until the Nth request message has been received.
func (a *mockBidiRequestAccessor) ExtractByIndex(i int) (any, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for {
		if i < len(*a.received) {
			return (*a.received)[i], true
		}
		if *a.recvDone {
			return nil, false
		}
		a.cond.Wait()
	}
}

// mockBidiResponseAccessor provides access to already-sent response messages.
type mockBidiResponseAccessor struct {
	sent []*grpcprotocol.ProtoMessageYAMLMarshaler
}

// ExtractByKey implements query.KeyExtractor interface.
func (a *mockBidiResponseAccessor) ExtractByKey(key string) (any, bool) {
	if key == messagesKey {
		return a.sent, true
	}
	return nil, false
}

type request struct {
	service  string
	method   string
	metadata *yamlutil.MDMarshaler
	message  any
	messages any
}

type expect struct {
	Service  *string       `yaml:"service"`
	Method   *string       `yaml:"method"`
	Metadata yaml.MapSlice `yaml:"metadata"`
	Message  any           `yaml:"message"`
	Messages []any         `yaml:"messages"`
}

func (e *expect) build(ctx *context.Context) (assert.Assertion, error) {
	var (
		serviceAssertion = assert.Nop()
		methodAssertion  = assert.Nop()
		err              error
	)
	if e.Service != nil {
		serviceAssertion, err = assert.Build(ctx.RequestContext(), *e.Service, assert.FromTemplate(ctx))
		if err != nil {
			return nil, errors.WrapPathf(err, "service", "invalid expect service")
		}
	}
	if e.Method != nil {
		methodAssertion, err = assert.Build(ctx.RequestContext(), *e.Method, assert.FromTemplate(ctx))
		if err != nil {
			return nil, errors.WrapPathf(err, "method", "invalid expect method")
		}
	}

	metadataAssertion, err := assertutil.BuildHeaderAssertion(ctx, e.Metadata)
	if err != nil {
		return nil, errors.WrapPathf(err, "metadata", "invalid expect metadata")
	}

	msgAssertion, err := assert.Build(ctx.RequestContext(), e.Message, assert.FromTemplate(ctx))
	if err != nil {
		return nil, errors.WrapPathf(err, "message", "invalid expect response message")
	}

	msgsAssertion, err := assert.Build(ctx.RequestContext(), e.Messages, assert.FromTemplate(ctx))
	if err != nil {
		return nil, errors.WrapPathf(err, "messages", "invalid expect response messages")
	}

	return assert.AssertionFunc(func(v any) error {
		req, ok := v.(*request)
		if !ok {
			return errors.Errorf("expected request but got %T", v)
		}
		if err := serviceAssertion.Assert(req.service); err != nil {
			return errors.WithPath(err, "service")
		}
		if err := methodAssertion.Assert(req.method); err != nil {
			return errors.WithPath(err, "method")
		}
		if err := metadataAssertion.Assert(req.metadata); err != nil {
			return errors.WithPath(err, "metadata")
		}
		if err := msgAssertion.Assert(req.message); err != nil {
			return errors.WithPath(err, "message")
		}
		if err := msgsAssertion.Assert(req.messages); err != nil {
			return errors.WithPath(err, "messages")
		}
		return nil
	}), nil
}

// Response represents an gRPC response.
type Response grpcprotocol.Expect

func (resp *Response) extract(msg proto.Message) (proto.Message, *status.Status, error) {
	if resp.Status.Code != "" {
		var code codes.Code
		c, err := strToCode(resp.Status.Code)
		if err != nil {
			return nil, nil, errors.WithPath(err, "status.code")
		}
		code = c

		smsg := code.String()
		if resp.Status.Message != "" {
			smsg = resp.Status.Message
		}

		if code != codes.OK {
			return nil, status.New(code, smsg), nil
		}
	}

	if resp.Message != nil {
		if err := grpcprotocol.ConvertToProto(resp.Message, msg); err != nil {
			return nil, nil, errors.WrapPath(err, "message", "invalid message")
		}
	}

	return msg, nil, nil
}

func (resp *Response) extractMessages(sctx *context.Context, method protoreflect.MethodDescriptor) ([]proto.Message, error) {
	var msgs []proto.Message
	for i, m := range resp.Messages {
		x, err := sctx.ExecuteTemplate(m)
		if err != nil {
			return nil, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to execute template")
		}
		msg := dynamicpb.NewMessage(method.Output())
		if x != nil {
			if err := grpcprotocol.ConvertToProto(x, msg); err != nil {
				return nil, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "invalid message")
			}
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func strToCode(s string) (codes.Code, error) {
	switch s {
	case "OK":
		return codes.OK, nil
	case "Canceled":
		return codes.Canceled, nil
	case "Unknown":
		return codes.Unknown, nil
	case "InvalidArgument":
		return codes.InvalidArgument, nil
	case "DeadlineExceeded":
		return codes.DeadlineExceeded, nil
	case "NotFound":
		return codes.NotFound, nil
	case "AlreadyExists":
		return codes.AlreadyExists, nil
	case "PermissionDenied":
		return codes.PermissionDenied, nil
	case "ResourceExhausted":
		return codes.ResourceExhausted, nil
	case "FailedPrecondition":
		return codes.FailedPrecondition, nil
	case "Aborted":
		return codes.Aborted, nil
	case "OutOfRange":
		return codes.OutOfRange, nil
	case "Unimplemented":
		return codes.Unimplemented, nil
	case "Internal":
		return codes.Internal, nil
	case "Unavailable":
		return codes.Unavailable, nil
	case "DataLoss":
		return codes.DataLoss, nil
	case "Unauthenticated":
		return codes.Unauthenticated, nil
	}
	if i, err := strconv.Atoi(s); err == nil {
		return intToCode(i)
	}
	return codes.Unknown, fmt.Errorf("invalid status code %q", s)
}

func intToCode(i int) (codes.Code, error) {
	if i > math.MaxUint32 {
		return 0, errors.Errorf("invalid status code %d: exceeds the maximum limit for uint32", i)
	}
	return codes.Code(i), nil
}
