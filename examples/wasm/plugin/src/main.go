package main

import (
	gocontext "context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"github.com/scenarigo/scenarigo/context"
	emptypb "github.com/scenarigo/scenarigo/examples/wasm/plugin/src/pb/empty"
	servicepb "github.com/scenarigo/scenarigo/examples/wasm/plugin/src/pb/service"
	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/schema"
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

func (s *server) Ping(_ gocontext.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *server) Echo(_ gocontext.Context, req *servicepb.EchoRequest) (*servicepb.EchoResponse, error) {
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
		X: 2,
		Y: "hello",
	}
)

type T struct {
	x any
}

func (t *T) SetValue(v any) bool {
	t.x = v
	return true
}

func (t *T) GetValue() any {
	return t.x
}

func NewT() *T {
	return new(T)
}

func Println(v any) int {
	fmt.Println(v)
	return 0
}

var (
	DumpVarsStep                      = plugin.StepFunc(dumpVarsStep)
	Join         plugin.LeftArrowFunc = &join{}
)

func dumpVarsStep(ctx *context.Context, step *schema.Step) *context.Context {
	b, err := yaml.Marshal(step.Vars)
	if err != nil {
		ctx.Reporter().Errorf("failed to marshal vars: %s", err)
	}
	ctx.Reporter().Log(string(b))
	return ctx
}

type join struct{}

type joinArg struct {
	Prefix string `yaml:"prefix"`
	Text   string `yaml:"text"`
	Suffix string `yaml:"suffix"`
}

func (_ *join) Exec(in interface{}) (interface{}, error) {
	arg, ok := in.(*joinArg)
	if !ok {
		return nil, errors.New("arg must be a joinArg")
	}
	return arg.Prefix + arg.Text + arg.Suffix, nil
}

func (_ *join) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var arg joinArg
	if err := unmarshal(&arg); err != nil {
		return nil, err
	}
	return &arg, nil
}

func Sleep(s string) (plugin.Step, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return nil, err
	}
	return plugin.StepFunc(func(ctx *context.Context, step *schema.Step) *context.Context {
		time.Sleep(d)
		ctx.Reporter().FailNow()
		return ctx
	}), nil
}

func SetVar(k string, v interface{}) (plugin.Step, error) {
	return plugin.StepFunc(func(ctx *context.Context, step *schema.Step) *context.Context {
		return ctx.WithVars(map[string]interface{}{k: v})
	}), nil
}

func PassContext(ctx *context.Context) bool {
	return true
}
