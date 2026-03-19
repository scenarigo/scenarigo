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

var (
	Int     int     = 1
	Int8    int8    = 2
	Int16   int16   = 3
	Int32   int32   = 4
	Int64   int64   = 5
	Uint    uint    = 6
	Uint8   uint8   = 7
	Uint16  uint16  = 8
	Uint32  uint32  = 9
	Uint64  uint64  = 10
	Float32 float32 = 11
	Float64 float64 = 12
	Uintptr uintptr = 13
	Bool    bool    = true
	String  string  = "hello"
	Bytes   []byte  = []byte("world")
	Map             = map[string]any{"a": "x", "b": 1}
	Slice           = []any{1, -2, 3.14, true, "hello"}
	Array           = [2]int64{1, 2}
	Struct          = struct {
		X int
		Y string
	}{
		X: 1,
		Y: "hello",
	}
	StructPtr = &struct {
		X int
		Y string
	}{
		X: 1,
		Y: "hello",
	}
	StructChain = struct {
		X struct {
			Y struct {
				Z int
			}
		}
	}{
		X: struct {
			Y struct {
				Z int
			}
		}{
			Y: struct {
				Z int
			}{
				Z: 10,
			},
		},
	}
	Any any = 1
)

func Bar() int {
	return 2
}

func main() {
	plugin.Register(
		func() []*plugin.Definition {
			return []*plugin.Definition{
				plugin.ToDefinition("Bar", Bar),
				plugin.ToDefinition("EchoClient", EchoClient),
				plugin.ToDefinition("Int", Int),
				plugin.ToDefinition("Int8", Int8),
				plugin.ToDefinition("Int16", Int16),
				plugin.ToDefinition("Int32", Int32),
				plugin.ToDefinition("Int64", Int64),
				plugin.ToDefinition("Uint", Uint),
				plugin.ToDefinition("Uint8", Uint8),
				plugin.ToDefinition("Uint16", Uint16),
				plugin.ToDefinition("Uint32", Uint32),
				plugin.ToDefinition("Uint64", Uint64),
				plugin.ToDefinition("Float32", Float32),
				plugin.ToDefinition("Float64", Float64),
				plugin.ToDefinition("Uintptr", Uintptr),
				plugin.ToDefinition("Bool", Bool),
				plugin.ToDefinition("String", String),
				plugin.ToDefinition("Bytes", Bytes),
				plugin.ToDefinition("Map", Map),
				plugin.ToDefinition("Slice", Slice),
				plugin.ToDefinition("Array", Array),
				plugin.ToDefinition("Struct", Struct),
				plugin.ToDefinition("StructPtr", StructPtr),
				plugin.ToDefinition("StructChain", StructChain),
				plugin.ToDefinition("Any", Any),
				plugin.ToDefinition("ServerAddr", ServerAddr),
			}
		},
		func() []*plugin.Definition {
			return []*plugin.Definition{
				plugin.ToDefinition("Bar", Bar),
				plugin.ToDefinition("EchoClient", EchoClient),
				plugin.ToDefinition("Int", Int),
				plugin.ToDefinition("Int8", Int8),
				plugin.ToDefinition("Int16", Int16),
				plugin.ToDefinition("Int32", Int32),
				plugin.ToDefinition("Int64", Int64),
				plugin.ToDefinition("Uint", Uint),
				plugin.ToDefinition("Uint8", Uint8),
				plugin.ToDefinition("Uint16", Uint16),
				plugin.ToDefinition("Uint32", Uint32),
				plugin.ToDefinition("Uint64", Uint64),
				plugin.ToDefinition("Float32", Float32),
				plugin.ToDefinition("Float64", Float64),
				plugin.ToDefinition("Uintptr", Uintptr),
				plugin.ToDefinition("Bool", Bool),
				plugin.ToDefinition("String", String),
				plugin.ToDefinition("Bytes", Bytes),
				plugin.ToDefinition("Map", Map),
				plugin.ToDefinition("Slice", Slice),
				plugin.ToDefinition("Array", Array),
				plugin.ToDefinition("Struct", Struct),
				plugin.ToDefinition("StructPtr", StructPtr),
				plugin.ToDefinition("StructChain", StructChain),
				plugin.ToDefinition("Any", Any),
				plugin.ToDefinition("ServerAddr", ServerAddr),
			}
		},
	)
}
