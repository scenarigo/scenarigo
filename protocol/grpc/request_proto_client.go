package grpc

import (
	gocontext "context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/grpc"
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
	conn           grpc.ClientConnInterface
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

// newProtoClientWithConn creates a protoClient using an existing connection.
// This is used when a custom client provides its connection for reflection.
func newProtoClientWithConn(ctx *context.Context, r *Request, conn grpc.ClientConnInterface) (*protoClient, error) {
	resolver := grpcproto.NewReflectionClient(ctx.RequestContext(), conn)

	sd, err := resolver.ResolveService(protoreflect.FullName(r.Service))
	if err != nil {
		if grpcproto.IsUnimplementedReflectionServiceError(err) {
			return nil, fmt.Errorf("server doesn't implement gRPC reflection service: %w", err)
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
