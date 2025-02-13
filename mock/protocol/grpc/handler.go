package grpc

import (
	gocontext "context"
	"fmt"
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

	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/assertutil"
	"github.com/scenarigo/scenarigo/internal/yamlutil"
	grpcprotocol "github.com/scenarigo/scenarigo/protocol/grpc"
)

func (s *server) convertToServicDesc(sd protoreflect.ServiceDescriptor) *grpc.ServiceDesc {
	desc := &grpc.ServiceDesc{
		ServiceName: string(sd.FullName()),
		Metadata:    sd.ParentFile().Path(),
	}
	for i := 0; i < sd.Methods().Len(); i++ {
		m := sd.Methods().Get(i)
		// TODO: streaming RPC
		// if m.IsStreamingServer() || m.IsStreamingClient() {
		// 	desc.Streams = append(desc.Streams, grpc.StreamDesc{
		// 		StreamName:    string(m.Name()),
		// 		ServerStreams: m.IsStreamingServer(),
		// 		ClientStreams: m.IsStreamingClient(),
		// 		Handler: func(srv any, stream grpc.ServerStream) error {
		// 			return nil
		// 		},
		// 	})
		// } else {
		desc.Methods = append(desc.Methods, grpc.MethodDesc{
			MethodName: string(m.Name()),
			Handler:    s.unaryHandler(sd.FullName(), m),
		})
		// }
	}
	return desc
}

func (s *server) unaryHandler(svcName protoreflect.FullName, method protoreflect.MethodDescriptor) func(srv any, ctx gocontext.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx gocontext.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		mock, err := s.iter.Next()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get mock: %s", err)
		}

		if mock.Protocol != "grpc" {
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

type request struct {
	service  string
	method   string
	metadata *yamlutil.MDMarshaler
	message  any
}

type expect struct {
	Service  *string       `yaml:"service"`
	Method   *string       `yaml:"method"`
	Metadata yaml.MapSlice `yaml:"metadata"`
	Message  any           `yaml:"message"`
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

	assertion, err := assert.Build(ctx.RequestContext(), e.Message, assert.FromTemplate(ctx))
	if err != nil {
		return nil, errors.WrapPathf(err, "message", "invalid expect response message")
	}

	return assert.AssertionFunc(func(v interface{}) error {
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
		if err := assertion.Assert(req.message); err != nil {
			return errors.WithPath(err, "message")
		}
		return nil
	}), nil
}

// Response represents an gRPC response.
type Response grpcprotocol.Expect

func (resp *Response) extract(msg proto.Message) (proto.Message, *status.Status, error) {
	if resp.Status.Code != "" {
		code := codes.OK
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
