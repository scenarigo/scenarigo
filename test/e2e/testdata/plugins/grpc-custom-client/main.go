package main

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"

	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

// customClientInterceptor adds a custom header to verify the custom client is being used.
func customClientInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ctx = metadata.AppendToOutgoingContext(ctx, "x-custom-client", "true")
	return invoker(ctx, method, req, reply, cc, opts...)
}

func init() {
	plugin.RegisterSetup(setup)
}

type echoServer struct {
	test.UnimplementedTestServer
}

func (s *echoServer) Echo(ctx context.Context, req *test.EchoRequest) (*test.EchoResponse, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get("x-custom-client"); len(values) > 0 {
			grpc.SetHeader(ctx, metadata.Pairs("x-custom-client-verified", values[0]))
		}
	}
	return &test.EchoResponse{
		MessageId:   req.GetMessageId(),
		MessageBody: req.GetMessageBody(),
	}, nil
}

// testClientWrapper wraps TestClient with connection for reflection support.
type testClientWrapper struct {
	test.TestClient
	conn grpc.ClientConnInterface
}

// ClientConn returns the underlying connection for reflection.
func (w *testClientWrapper) ClientConn() grpc.ClientConnInterface {
	return w.conn
}

var TestClient *testClientWrapper

func setup(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	s := grpc.NewServer()
	test.RegisterTestServer(s, &echoServer{})
	reflection.Register(s)

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		ctx.Reporter().Fatalf("unexpected error: %s", err)
	}

	go func() {
		_ = s.Serve(ln)
	}()

	cc, err := grpc.NewClient(ln.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(customClientInterceptor),
	)
	if err != nil {
		ctx.Reporter().Fatalf("failed to create client: %s", err)
	}

	TestClient = &testClientWrapper{
		TestClient: test.NewTestClient(cc),
		conn:       cc,
	}

	return ctx, func(ctx *plugin.Context) {
		cc.Close()
		s.GracefulStop()
	}
}
