package grpc

import (
	gocontext "context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	grpcproto "github.com/scenarigo/scenarigo/protocol/grpc/proto"
	"github.com/scenarigo/scenarigo/version"
)

var (
	defaultUserAgent = fmt.Sprintf("scenarigo/%s", version.String())
	connPool         = &grpcConnPool{
		conns: map[string]*grpc.ClientConn{},
	}
	fdCache = &protoFdCache{
		fds: map[string]grpcproto.FileDescriptors{},
	}
)

type grpcConnPool struct {
	m     sync.Mutex
	conns map[string]*grpc.ClientConn
}

func (p *grpcConnPool) NewClient(target string, o *AuthOption) (*grpc.ClientConn, error) {
	b, err := json.Marshal(o)
	if err != nil {
		return nil, errors.WrapPath(err, "auth", "failed to marshal auth option")
	}
	k := fmt.Sprintf("target=%s:auth=%s", target, string(b))

	p.m.Lock()
	defer p.m.Unlock()
	if conn, ok := p.conns[k]; ok {
		return conn, nil
	}
	creds, err := o.Credentials()
	if err != nil {
		return nil, errors.WithPath(err, "auth")
	}
	conn, err := grpc.NewClient(target, grpc.WithUserAgent(defaultUserAgent), grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, errors.WithPath(err, "target")
	}
	p.conns[k] = conn
	return conn, nil
}

func (p *grpcConnPool) closeConnection(target string) error {
	prefix := fmt.Sprintf("target=%s:", target)
	p.m.Lock()
	defer p.m.Unlock()
	for k, conn := range p.conns {
		if strings.HasPrefix(k, prefix) {
			delete(p.conns, k)
			if err := conn.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

type protoFdCache struct {
	m   sync.Mutex
	fds map[string]grpcproto.FileDescriptors
}

func (c *protoFdCache) Compile(ctx gocontext.Context, imports, files []string) (grpcproto.FileDescriptors, error) {
	k := fmt.Sprintf("imports=%s:files=%s", strings.Join(imports, ","), strings.Join(files, ","))

	c.m.Lock()
	defer c.m.Unlock()
	if fds, ok := c.fds[k]; ok {
		return fds, nil
	}
	fds, err := grpcproto.NewCompiler(imports).Compile(ctx, files)
	if err != nil {
		return nil, err
	}
	c.fds[k] = fds
	return fds, nil
}

type protoClient struct {
	r              *Request
	conn           *grpc.ClientConn
	resolver       grpcproto.ServiceDescriptorResolver
	fullMethodName string
	md             protoreflect.MethodDescriptor
}

func newProtoClient(ctx *context.Context, r *Request, opts *RequestOptions) (*protoClient, error) {
	if r.Target == "" {
		return nil, errors.ErrorPath("target", "target must be specified")
	}
	x, err := ctx.ExecuteTemplate(r.Target)
	if err != nil {
		return nil, errors.WrapPath(err, "target", "invalid target")
	}
	target, ok := x.(string)
	if !ok {
		return nil, errors.ErrorPathf("target", "target must be string but %T", x)
	}
	conn, err := connPool.NewClient(target, opts.Auth)
	if err != nil {
		return nil, err
	}

	var resolver grpcproto.ServiceDescriptorResolver
	if !opts.Reflection.IsEnabled() && opts.Proto != nil && len(opts.Proto.Files) > 0 {
		fds, err := fdCache.Compile(ctx.RequestContext(), opts.Proto.Imports, opts.Proto.Files)
		if err != nil {
			return nil, errors.WithPath(err, "options.proto")
		}
		resolver = fds
	}
	if resolver == nil {
		resolver = grpcproto.NewReflectionClient(ctx.RequestContext(), conn)
	}

	sd, err := resolver.ResolveService(protoreflect.FullName(r.Service))
	if err != nil {
		if grpcproto.IsUnimplementedReflectionServiceError(err) {
			return nil, fmt.Errorf("%s doesn't implement gRPC reflection service: %w", target, err)
		}
		return nil, errors.WithPath(err, "service")
	}
	md := sd.Methods().ByName(protoreflect.Name(r.Method))
	if md == nil {
		return nil, errors.ErrorPathf("method", "method %q not found", r.Method)
	}

	return &protoClient{
		r:              r,
		conn:           conn,
		resolver:       resolver,
		fullMethodName: fmt.Sprintf("/%s/%s", sd.FullName(), md.Name()),
		md:             md,
	}, nil
}

func (client *protoClient) buildRequestMessage(ctx *context.Context) (proto.Message, error) {
	in := dynamicpb.NewMessage(client.md.Input())
	if err := buildRequestMsg(ctx, in, client.r.Message); err != nil {
		return nil, errors.WrapPathf(err, "message", "failed to build request message")
	}
	return in, nil
}

func (client *protoClient) invoke(ctx gocontext.Context, in proto.Message, opts ...grpc.CallOption) (proto.Message, *status.Status, error) {
	out := dynamicpb.NewMessage(client.md.Output())
	var sts *status.Status
	if err := client.conn.Invoke(ctx, client.fullMethodName, in, out, opts...); err != nil {
		sts = status.Convert(err)
	}
	return out, sts, nil
}

func (client *protoClient) isStreamingClient() bool {
	return client.md.IsStreamingClient()
}

func (client *protoClient) isStreamingServer() bool {
	return client.md.IsStreamingServer()
}

func (client *protoClient) buildRequestMessages(ctx *context.Context) ([]proto.Message, error) {
	msgs := make([]proto.Message, len(client.r.Messages))
	for i, m := range client.r.Messages {
		in := dynamicpb.NewMessage(client.md.Input())
		if err := buildRequestMsg(ctx, in, m); err != nil {
			return nil, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to build request message")
		}
		msgs[i] = in
	}
	return msgs, nil
}

func (client *protoClient) invokeServerStream(ctx gocontext.Context, in proto.Message, opts ...grpc.CallOption) ([]proto.Message, metadata.MD, metadata.MD, *status.Status, error) {
	streamDesc := &grpc.StreamDesc{
		ServerStreams: true,
	}
	stream, err := client.conn.NewStream(ctx, streamDesc, client.fullMethodName, opts...)
	if err != nil {
		return nil, nil, nil, status.Convert(err), nil
	}
	if err := stream.SendMsg(in); err != nil {
		return nil, nil, nil, status.Convert(err), nil
	}
	if err := stream.CloseSend(); err != nil {
		return nil, nil, nil, status.Convert(err), nil
	}

	var msgs []proto.Message
	for {
		out := dynamicpb.NewMessage(client.md.Output())
		if err := stream.RecvMsg(out); err != nil {
			if stderrors.Is(err, io.EOF) {
				break
			}
			header, _ := stream.Header()
			return nil, header, stream.Trailer(), status.Convert(err), nil
		}
		msgs = append(msgs, out)
	}

	header, _ := stream.Header()
	return msgs, header, stream.Trailer(), nil, nil
}

func (client *protoClient) invokeClientStream(ctx gocontext.Context, msgs []proto.Message, opts ...grpc.CallOption) (proto.Message, metadata.MD, metadata.MD, *status.Status, error) {
	streamDesc := &grpc.StreamDesc{
		ClientStreams: true,
	}
	stream, err := client.conn.NewStream(ctx, streamDesc, client.fullMethodName, opts...)
	if err != nil {
		return nil, nil, nil, status.Convert(err), nil
	}
	for _, msg := range msgs {
		if err := stream.SendMsg(msg); err != nil {
			return nil, nil, nil, status.Convert(err), nil
		}
	}
	if err := stream.CloseSend(); err != nil {
		return nil, nil, nil, status.Convert(err), nil
	}

	out := dynamicpb.NewMessage(client.md.Output())
	if err := stream.RecvMsg(out); err != nil {
		header, _ := stream.Header()
		return nil, header, stream.Trailer(), status.Convert(err), nil
	}

	header, _ := stream.Header()
	return out, header, stream.Trailer(), nil, nil
}

func (client *protoClient) invokeBidiStream(ctx gocontext.Context, sCtx *context.Context, opts ...grpc.CallOption) ([]proto.Message, metadata.MD, metadata.MD, *status.Status, error) {
	streamDesc := &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}
	stream, err := client.conn.NewStream(ctx, streamDesc, client.fullMethodName, opts...)
	if err != nil {
		return nil, nil, nil, status.Convert(err), nil
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
			out := dynamicpb.NewMessage(client.md.Output())
			if err := stream.RecvMsg(out); err != nil {
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
			msgs = append(msgs, out)
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
		in := dynamicpb.NewMessage(client.md.Input())
		if x != nil {
			if err := ConvertToProto(x, in); err != nil {
				return nil, nil, nil, nil, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to build request message")
			}
		}
		if err := stream.SendMsg(in); err != nil {
			return nil, nil, nil, status.Convert(err), nil
		}
		bidiReq.sent = append(bidiReq.sent, in)
	}

	if err := stream.CloseSend(); err != nil {
		return nil, nil, nil, status.Convert(err), nil
	}

	// Wait for all responses
	result := <-recvCh
	if recvErr := result.err; recvErr != nil {
		header, _ := stream.Header()
		return nil, header, stream.Trailer(), status.Convert(recvErr), nil
	}

	header, _ := stream.Header()
	return result.msgs, header, stream.Trailer(), nil, nil
}

// bidiRequestAccessor provides access to already-sent request messages.
type bidiRequestAccessor struct {
	sent []proto.Message
}

// ExtractByKey implements query.KeyExtractor interface.
func (a *bidiRequestAccessor) ExtractByKey(key string) (any, bool) {
	if key == "messages" {
		msgs := make([]*ProtoMessageYAMLMarshaler, len(a.sent))
		for i, m := range a.sent {
			msgs[i] = &ProtoMessageYAMLMarshaler{m}
		}
		return msgs, true
	}
	return nil, false
}

// bidiResponseAccessor provides access to streaming responses with blocking semantics.
// When accessing messages[N], it blocks until the Nth response has been received.
type bidiResponseAccessor struct {
	mu       *sync.Mutex
	cond     *sync.Cond
	received *[]proto.Message
	recvDone *bool
}

// ExtractByKey implements query.KeyExtractorContext interface.
func (a *bidiResponseAccessor) ExtractByKey(_ gocontext.Context, key string) (any, bool) {
	if key == "messages" {
		return a, true
	}
	return nil, false
}

// ExtractByIndex implements query.IndexExtractor interface.
func (a *bidiResponseAccessor) ExtractByIndex(i int) (any, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for {
		if i < len(*a.received) {
			return &ProtoMessageYAMLMarshaler{(*a.received)[i]}, true
		}
		if *a.recvDone {
			return nil, false
		}
		a.cond.Wait()
	}
}
