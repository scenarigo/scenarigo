package main

import (
	"context"
	"errors"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"github.com/scenarigo/scenarigo/plugin"

	emptypb "github.com/scenarigo/scenarigo/examples/grpc/plugin/src/pb/empty"
	servicepb "github.com/scenarigo/scenarigo/examples/grpc/plugin/src/pb/service"
)

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

func (s *server) Ping(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *server) Echo(_ context.Context, req *servicepb.EchoRequest) (*servicepb.EchoResponse, error) {
	return &servicepb.EchoResponse{
		MessageId:   req.GetMessageId(),
		MessageBody: req.GetMessageBody(),
	}, nil
}

var (
	PingClient servicepb.PingClient
	EchoClient servicepb.EchoClient
)

func createClients(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	cc, err := grpc.NewClient(ServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		ctx.Reporter().Fatalf("failed to create Ping client: %s", err)
	}
	PingClient = servicepb.NewPingClient(cc)
	EchoClient = servicepb.NewEchoClient(cc)
	return ctx, func(ctx *plugin.Context) {
		cc.Close()
	}
}
