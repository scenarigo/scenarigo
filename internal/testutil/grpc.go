package testutil

import (
	"context"
	"net"
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

func TestGRPCServerFunc(f func(context.Context, *testpb.EchoRequest) (*testpb.EchoResponse, error)) testpb.TestServer {
	return testServer(f)
}
