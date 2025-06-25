package main

import (
	"context"
	"errors"
	"net"

	"github.com/goccy/wasi-go-net/wasip1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	servicepb "test/pb/service"

	"github.com/scenarigo/scenarigo/plugin"
)

func init() {
	plugin.RegisterSetup(setupServer)
	plugin.RegisterSetup(setupClient)
}

var (
	EchoClient servicepb.EchoClient
	ServerAddr string
)

type server struct{}

func (s *server) Echo(_ context.Context, req *servicepb.EchoRequest) (*servicepb.EchoResponse, error) {
	return &servicepb.EchoResponse{
		MessageId:   req.GetMessageId(),
		MessageBody: req.GetMessageBody(),
	}, nil
}

func setupServer(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	s := grpc.NewServer()
	srv := &server{}
	servicepb.RegisterEchoServer(s, srv)
	reflection.Register(s)

	ln, err := wasip1.Listen("tcp", "localhost:0")
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

func setupClient(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	cc, err := grpc.NewClient(
		ServerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, address string) (net.Conn, error) {
			return wasip1.DialContext(ctx, "tcp", address)
		}),
	)
	if err != nil {
		ctx.Reporter().Fatalf("failed to create grpc client: %s", err)
	}
	EchoClient = servicepb.NewEchoClient(cc)
	return ctx, func(ctx *plugin.Context) {
		cc.Close()
	}
}

var Foo int = 1

func Bar() int {
	return 2
}

func main() {
	plugin.Register(
		func() []*plugin.Definition {
			return []*plugin.Definition{
				plugin.ToDefinition("Bar", Bar),
				plugin.ToDefinition("EchoClient", EchoClient),
				plugin.ToDefinition("Foo", Foo),
				plugin.ToDefinition("ServerAddr", ServerAddr),
			}
		},
		func() []*plugin.Definition {
			return []*plugin.Definition{
				plugin.ToDefinition("Bar", Bar),
				plugin.ToDefinition("EchoClient", EchoClient),
				plugin.ToDefinition("Foo", Foo),
				plugin.ToDefinition("ServerAddr", ServerAddr),
			}
		},
	)
}
