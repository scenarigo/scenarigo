package grpc

import (
	gocontext "context"
	stderrors "errors"
	"fmt"
	"io"
	"math"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/goccy/go-yaml"
	query "github.com/zoncoen/query-go/v2"

	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/assertutil"
	"github.com/scenarigo/scenarigo/internal/grpcstream"
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
		// Expose the received request to response templates as request.message,
		// mirroring the scenario-side dump shape and the streaming handlers.
		sctx := context.New(nil).WithRequest(&mockRequestAccessor{message: req})
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

	// Expose the received request to response templates as request.message,
	// mirroring the scenario-side dump shape.
	sctx := context.New(nil)
	sctx = sctx.WithRequest(&mockRequestAccessor{message: req})

	// Resolve the configured status up front (a malformed code or template is a
	// config error), but apply a non-OK status only after the configured messages
	// have been sent: a gRPC stream may emit messages and then end with an error.
	statusErr, err := configuredStatusError(sctx, resp)
	if err != nil {
		return err
	}

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
	return statusErr
}

// configuredStatusError evaluates the templates in resp.Status and returns the
// non-OK status error to end the stream with, or nil when no status (or
// codes.OK) is configured. The second return value is a config error
// (Internal) for a template failure or a malformed status code. It takes the
// same template context as the response messages so that a templated status
// resolves consistently with the unary handler, which template-executes the
// whole response.
func configuredStatusError(sctx *context.Context, resp *Response) (error, error) {
	v, err := sctx.ExecuteTemplate(resp.Status)
	if err != nil {
		return nil, status.Error(codes.Internal, errors.WrapPath(err, "response.status", "failed to execute template").Error())
	}
	st, ok := v.(grpcprotocol.ExpectStatus)
	if !ok {
		return nil, status.Error(codes.Internal, errors.WithPath(fmt.Errorf("failed to execute template of status: unexpected type %T", v), "response.status").Error())
	}
	if st.Code == "" {
		return nil, nil //nolint:nilnil // no status configured and no error
	}
	code, err := strToCode(st.Code)
	if err != nil {
		return nil, status.Error(codes.Internal, errors.WithPath(err, "response.status.code").Error())
	}
	if code == codes.OK {
		return nil, nil //nolint:nilnil // OK status is the same as no status
	}
	smsg := code.String()
	if st.Message != "" {
		smsg = st.Message
	}
	return status.Error(code, smsg), nil
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

	// Set up template context with request.messages
	sctx := context.New(nil)
	sctx = sctx.WithRequest(&clientStreamRequestAccessor{received: msgs})

	// A client-streaming RPC ends with a single response or an error status, so a
	// non-OK status replaces the response: return it before sending.
	if statusErr, err := configuredStatusError(sctx, resp); err != nil {
		return err
	} else if statusErr != nil {
		return statusErr
	}

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
	// Accumulate requests in the background. Blocking request references
	// (request.messages[N]) wait on the buffer, bounded by the template
	// evaluation context, so a deadlocked scenario fails instead of hanging.
	//
	// The goroutine is released when this handler returns: gRPC cancels
	// stream.Context() on return, which unblocks stream.RecvMsg. recvCh is
	// buffered so the goroutine never blocks on send even if we return early
	// (e.g. on a mid-stream send error) without draining it.
	buf := grpcstream.NewBuffer[*grpcprotocol.ProtoMessageYAMLMarshaler]()
	recvCh := make(chan error, 1)
	go func() {
		for {
			req := dynamicpb.NewMessage(method.Input())
			if err := stream.RecvMsg(req); err != nil {
				buf.Close()
				if stderrors.Is(err, io.EOF) {
					err = nil
				}
				recvCh <- err
				return
			}
			buf.Append(&grpcprotocol.ProtoMessageYAMLMarshaler{Message: req})
		}
	}()

	// Set up blocking request accessor for template evaluation
	bidiReq := &mockBidiRequestAccessor{buf: buf}

	// Set up response accessor for referencing already-sent responses
	bidiResp := &mockBidiResponseAccessor{}

	// Bind the stream context (which carries any client-propagated deadline) so
	// blocking request references are bounded by it.
	sctx := context.New(nil).WithRequestContext(stream.Context())
	sctx = sctx.WithRequest(bidiReq)
	sctx = sctx.WithResponse(bidiResp)

	list, err := resp.messageList()
	if err != nil {
		return status.Error(codes.Internal, errors.WithPath(err, "response").Error())
	}
	// Send response messages sequentially, evaluating templates as we go. Each
	// evaluation is bounded by the client-propagated deadline, or by the
	// deadlock guard when none is set, so a blocking request reference that can
	// never be satisfied fails instead of hanging. Putting the guard on the
	// context lets the template engine report an interrupted wait as an error
	// instead of treating it as an undefined value.
	for i, m := range list {
		evalCtx := sctx
		cancelEval := gocontext.CancelFunc(func() {})
		if _, ok := sctx.RequestContext().Deadline(); !ok {
			c, cancel := gocontext.WithTimeout(sctx.RequestContext(), grpcstream.DefaultMessageWaitTimeout)
			evalCtx, cancelEval = sctx.WithRequestContext(c), cancel
		}
		x, err := evalCtx.ExecuteTemplate(m)
		evalErr := evalCtx.RequestContext().Err()
		cancelEval()
		if err != nil {
			// Distinguish the interruption causes: only an expired deadline
			// suggests a deadlock, while a cancellation (e.g. the client
			// disconnecting) is an external abort.
			switch {
			case stderrors.Is(evalErr, gocontext.DeadlineExceeded):
				return status.Error(codes.DeadlineExceeded, errors.WrapPathf(err, fmt.Sprintf("response.messages[%d]", i), "interrupted while waiting for a streaming request message (possible deadlock or timeout)").Error())
			case evalErr != nil:
				return status.Error(codes.Canceled, errors.WrapPathf(err, fmt.Sprintf("response.messages[%d]", i), "canceled while waiting for a streaming request message").Error())
			}
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
	if recvErr := <-recvCh; recvErr != nil {
		return status.Error(codes.Internal, errors.WrapPath(recvErr, "expect", "failed to receive messages").Error())
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
		messages: buf.Snapshot(),
	}); err != nil {
		return status.Error(codes.InvalidArgument, errors.WrapPath(err, "expect", "request assertion failed").Error())
	}

	// Resolve the configured status only now: a bidi status template may
	// reference request.messages[N], which is guaranteed to be non-blocking once
	// the whole request stream has been received. The non-OK status is applied
	// after the response messages and the request assertion, since a gRPC stream
	// may emit messages and then end with an error.
	statusErr, err := configuredStatusError(sctx, resp)
	if err != nil {
		return err
	}
	return statusErr
}

// clientStreamRequestAccessor provides access to received client-stream request messages.
// mockRequestAccessor exposes the received request to response templates as
// request.message, mirroring the scenario-side dump shape for unary and
// server-streaming methods (client/bidi streaming expose request.messages via
// their own accessors).
type mockRequestAccessor struct {
	message proto.Message
}

var _ query.KeyExtractor = (*mockRequestAccessor)(nil)

// ExtractByKey implements query.KeyExtractor interface.
func (a *mockRequestAccessor) ExtractByKey(_ gocontext.Context, key string) (any, error) {
	if key == "message" {
		return a.message, nil
	}
	return nil, query.ErrNotFound
}

type clientStreamRequestAccessor struct {
	received []*grpcprotocol.ProtoMessageYAMLMarshaler
}

var _ query.KeyExtractor = (*clientStreamRequestAccessor)(nil)

// ExtractByKey implements query.KeyExtractor interface.
func (a *clientStreamRequestAccessor) ExtractByKey(_ gocontext.Context, key string) (any, error) {
	if key == messagesKey {
		return a.received, nil
	}
	return nil, query.ErrNotFound
}

// mockBidiRequestAccessor provides blocking access to request messages received by a background goroutine.
type mockBidiRequestAccessor struct {
	buf *grpcstream.Buffer[*grpcprotocol.ProtoMessageYAMLMarshaler]
}

var (
	_ query.KeyExtractor   = (*mockBidiRequestAccessor)(nil)
	_ query.IndexExtractor = (*mockBidiRequestAccessor)(nil)
)

// ExtractByKey implements query.KeyExtractor interface.
func (a *mockBidiRequestAccessor) ExtractByKey(_ gocontext.Context, key string) (any, error) {
	if key == messagesKey {
		return a, nil
	}
	return nil, query.ErrNotFound
}

// ExtractByIndex implements query.IndexExtractor interface. It blocks
// until the Nth request message has been received, bounded by ctx.
func (a *mockBidiRequestAccessor) ExtractByIndex(ctx gocontext.Context, i int) (any, error) {
	msg, ok := a.buf.At(ctx, i)
	if !ok {
		return nil, query.ErrNotFound
	}
	return msg, nil
}

// MarshalYAML implements the yaml.InterfaceMarshaler interface. An unindexed
// {{request.messages}} reference materializes as the messages received so far
// instead of rendering the accessor struct itself.
func (a *mockBidiRequestAccessor) MarshalYAML() (any, error) {
	return a.buf.Snapshot(), nil
}

// mockBidiResponseAccessor provides access to already-sent response messages.
type mockBidiResponseAccessor struct {
	sent []*grpcprotocol.ProtoMessageYAMLMarshaler
}

var _ query.KeyExtractor = (*mockBidiResponseAccessor)(nil)

// ExtractByKey implements query.KeyExtractor interface.
func (a *mockBidiResponseAccessor) ExtractByKey(_ gocontext.Context, key string) (any, error) {
	if key == messagesKey {
		return a.sent, nil
	}
	return nil, query.ErrNotFound
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

// messageList returns the configured response messages as a list. The mock
// sends these, so they must be authored as a literal list (unlike the
// client-side expect, whose messages may be an assertion such as contains);
// any other non-nil value is a configuration error rather than an empty
// stream, so it is reported instead of being silently ignored.
func (resp *Response) messageList() ([]any, error) {
	if resp.Messages == nil {
		return nil, nil
	}
	l, ok := resp.Messages.([]any)
	if !ok {
		return nil, errors.ErrorPathf("messages", "must be a list of messages but got %T", resp.Messages)
	}
	return l, nil
}

func (resp *Response) extractMessages(sctx *context.Context, method protoreflect.MethodDescriptor) ([]proto.Message, error) {
	list, err := resp.messageList()
	if err != nil {
		return nil, err
	}
	var msgs []proto.Message
	for i, m := range list {
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
	if i < 0 || i > math.MaxUint32 {
		return 0, errors.Errorf("invalid status code %d: out of range for uint32", i)
	}
	return codes.Code(i), nil
}
