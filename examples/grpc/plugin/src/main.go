package main

import (
	"context"
	"errors"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"

	"github.com/scenarigo/scenarigo/plugin"

	emptypb "github.com/scenarigo/scenarigo/examples/grpc/plugin/src/pb/empty"
	servicepb "github.com/scenarigo/scenarigo/examples/grpc/plugin/src/pb/service"
)

// customClientInterceptor adds a custom header to verify the custom client is being used.
func customClientInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	// Add custom metadata to verify custom client is used
	ctx = metadata.AppendToOutgoingContext(ctx, "x-custom-client", "true")
	return invoker(ctx, method, req, reply, cc, opts...)
}

func init() {
	plugin.RegisterSetup(startServer)
	plugin.RegisterSetup(startTLSServer)
	plugin.RegisterSetup(createClients)
}

var (
	ServerAddr     string
	TLSServerAddr  string
	TLSCertificate string
)

func startServer(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	s := grpc.NewServer()
	srv := &server{}
	servicepb.RegisterPingServer(s, srv)
	servicepb.RegisterEchoServer(s, srv)
	reflection.Register(s)

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		ctx.Reporter().Fatalf("unexpected error: %s", err)
	}
	ServerAddr = ln.Addr().String()

	go func() {
		if err := s.Serve(ln); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			ctx.Reporter().Errorf("failed to start server: %s", err)
		}
	}()

	return ctx, func(ctx *plugin.Context) {
		s.GracefulStop()
	}
}

func startTLSServer(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	tmp, err := os.MkdirTemp("", "scenarigo-example-")
	if err != nil {
		ctx.Reporter().Fatalf("failed to create a temporary directory: %s", err)
	}
	caCert, serverCert, serverKey, err := generateCert(tmp)
	if err != nil {
		ctx.Reporter().Fatalf("failed to create certificates: %s", err)
	}
	TLSCertificate = caCert
	creds, err := credentials.NewServerTLSFromFile(serverCert, serverKey)
	if err != nil {
		ctx.Reporter().Fatalf("failed to create a server TLS credential: %s", err)
	}

	s := grpc.NewServer(grpc.Creds(creds))
	srv := &server{}
	servicepb.RegisterPingServer(s, srv)
	servicepb.RegisterEchoServer(s, srv)
	reflection.Register(s)

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		ctx.Reporter().Fatalf("unexpected error: %s", err)
	}
	TLSServerAddr = ln.Addr().String()

	go func() {
		if err := s.Serve(ln); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			ctx.Reporter().Errorf("failed to start server: %s", err)
		}
	}()

	return ctx, func(ctx *plugin.Context) {
		s.GracefulStop()
		os.RemoveAll(tmp)
	}
}

type server struct{}

func (s *server) Ping(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	// Echo back the custom client header if present
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get("x-custom-client"); len(values) > 0 {
			grpc.SetHeader(ctx, metadata.Pairs("x-custom-client-verified", values[0]))
		}
	}
	return &emptypb.Empty{}, nil
}

func (s *server) Echo(ctx context.Context, req *servicepb.EchoRequest) (*servicepb.EchoResponse, error) {
	// Echo back the custom client header if present
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get("x-custom-client"); len(values) > 0 {
			grpc.SetHeader(ctx, metadata.Pairs("x-custom-client-verified", values[0]))
		}
	}
	return &servicepb.EchoResponse{
		MessageId:   req.GetMessageId(),
		MessageBody: req.GetMessageBody(),
	}, nil
}

// pingClientWrapper wraps PingClient with connection for reflection support.
type pingClientWrapper struct {
	servicepb.PingClient
	conn grpc.ClientConnInterface
}

// ClientConn returns the underlying connection for reflection.
func (w *pingClientWrapper) ClientConn() grpc.ClientConnInterface {
	return w.conn
}

// echoClientWrapper wraps EchoClient with connection for reflection support.
type echoClientWrapper struct {
	servicepb.EchoClient
	conn grpc.ClientConnInterface
}

// ClientConn returns the underlying connection for reflection.
func (w *echoClientWrapper) ClientConn() grpc.ClientConnInterface {
	return w.conn
}

var (
	PingClient *pingClientWrapper
	EchoClient *echoClientWrapper
)

func createClients(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	cc, err := grpc.NewClient(ServerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(customClientInterceptor),
	)
	if err != nil {
		ctx.Reporter().Fatalf("failed to create Ping client: %s", err)
	}
	PingClient = &pingClientWrapper{
		PingClient: servicepb.NewPingClient(cc),
		conn:       cc,
	}
	EchoClient = &echoClientWrapper{
		EchoClient: servicepb.NewEchoClient(cc),
		conn:       cc,
	}
	return ctx, func(ctx *plugin.Context) {
		cc.Close()
	}
}
