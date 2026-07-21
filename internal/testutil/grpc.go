package testutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

func StartTestGRPCServer(t *testing.T, srv testpb.TestServer, optFuncs ...TestGRPCServerOption) string {
	t.Helper()

	var opts grpcServerOpts
	for _, f := range optFuncs {
		f(&opts)
	}

	var serverOpts []grpc.ServerOption
	if opts.tls != nil {
		creds, err := credentials.NewServerTLSFromFile(opts.tls.certificate, opts.tls.key)
		if err != nil {
			t.Fatal(err)
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
	}

	s := grpc.NewServer(serverOpts...)
	testpb.RegisterTestServer(s, srv)
	if opts.reflection {
		reflection.Register(s)
	}

	ln, err := net.Listen("tcp", "localhost:0") //nolint:noctx // no context available in test helper
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	t.Cleanup(func() {
		ln.Close()
	})

	go func() {
		err = s.Serve(ln)
	}()
	t.Cleanup(func() {
		s.Stop()
	})

	return ln.Addr().String()
}

type TestGRPCServerOption func(*grpcServerOpts)

type grpcServerOpts struct {
	reflection bool
	tls        *tlsConfig
}

type tlsConfig struct {
	certificate string
	key         string
}

func EnableReflection() TestGRPCServerOption {
	return func(opts *grpcServerOpts) {
		opts.reflection = true
	}
}

func EnableTLS(cert, key string) TestGRPCServerOption {
	return func(opts *grpcServerOpts) {
		opts.tls = &tlsConfig{
			certificate: cert,
			key:         key,
		}
	}
}

type testServer func(context.Context, *testpb.EchoRequest) (*testpb.EchoResponse, error)

func (f testServer) Echo(ctx context.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
	return f(ctx, req)
}

func (f testServer) ServerStreamEcho(req *testpb.EchoRequest, stream testpb.Test_ServerStreamEchoServer) error {
	resp, err := f(stream.Context(), req)
	if err != nil {
		return err
	}
	// Send the response multiple times (echo with index suffix)
	for i := range 3 {
		r := &testpb.EchoResponse{
			MessageId:   resp.GetMessageId(),
			MessageBody: fmt.Sprintf("%s-%d", resp.GetMessageBody(), i),
		}
		if err := stream.Send(r); err != nil {
			return err
		}
	}
	return nil
}

func (f testServer) ClientStreamEcho(stream testpb.Test_ClientStreamEchoServer) error {
	var bodies []string
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&testpb.EchoResponse{
				MessageId:   "aggregated",
				MessageBody: strings.Join(bodies, ","),
			})
		}
		if err != nil {
			return err
		}
		bodies = append(bodies, req.GetMessageBody())
	}
}

func (f testServer) BidiStreamEcho(stream testpb.Test_BidiStreamEchoServer) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		resp := &testpb.EchoResponse{
			MessageId:   req.GetMessageId(),
			MessageBody: fmt.Sprintf("re: %s", req.GetMessageBody()),
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

func TestGRPCServerFunc(f func(context.Context, *testpb.EchoRequest) (*testpb.EchoResponse, error)) testpb.TestServer {
	return testServer(f)
}
