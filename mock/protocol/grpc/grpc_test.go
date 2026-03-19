package grpc

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/scenarigo/scenarigo/logger"
	"github.com/scenarigo/scenarigo/mock/protocol"
	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

func init() {
	Register()
}

func TestGRPC_Server(t *testing.T) {
	cfg := `
proto:
  files:
  - ./testdata/test.proto
`

	tests := map[string]struct {
		filename string
		config   string
		f        func(*testing.T, string)
	}{
		"empty": {
			filename: "testdata/empty.yaml",
			f: func(t *testing.T, addr string) {
				t.Helper()
				c, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					t.Fatalf("failed to connect server: %s", err)
				}
				client := healthpb.NewHealthClient(c)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{
					Service: healthpb.Health_ServiceDesc.ServiceName,
				})
				if err != nil {
					t.Fatal(err)
				}
				if got, expect := resp.GetStatus(), healthpb.HealthCheckResponse_SERVING; got != expect {
					t.Errorf("expect %d but got %d", expect, got)
				}
			},
		},
		"success": {
			filename: "testdata/grpc.yaml",
			config:   cfg,
			f:        sendEchoRequest(nil, "1", "hello"),
		},
		"int status code": {
			filename: "testdata/int-status-code.yaml",
			config:   cfg,
			f:        sendEchoRequest(nil, "1", "hello"),
		},
		"no expect and response": {
			filename: "testdata/no-expect-response.yaml",
			config:   cfg,
			f:        sendEchoRequest(nil, "", ""),
		},
		"unauthenticated": {
			filename: "testdata/unauthenticated.yaml",
			config:   cfg,
			f:        sendEchoRequest(status.New(codes.Unauthenticated, "Unauthenticated"), "", ""),
		},
		"invalid expect service": {
			filename: "testdata/invalid-expect-service.yaml",
			config:   cfg,
			f:        sendEchoRequest(status.New(codes.InvalidArgument, ".expect.service: request assertion failed"), "", ""),
		},
		"invalid expect method": {
			filename: "testdata/invalid-expect-method.yaml",
			config:   cfg,
			f:        sendEchoRequest(status.New(codes.InvalidArgument, ".expect.method: request assertion failed"), "", ""),
		},
		"invalid expect metadata": {
			filename: "testdata/invalid-expect-metadata.yaml",
			config:   cfg,
			f:        sendEchoRequest(status.New(codes.InvalidArgument, ".expect.metadata.content-type: request assertion failed"), "", ""),
		},
		"invalid expect message": {
			filename: "testdata/invalid-expect-metadata.yaml",
			config:   cfg,
			f:        sendEchoRequest(status.New(codes.InvalidArgument, ".expect.metadata.content-type: request assertion failed"), "", ""),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p := protocol.Get("grpc")
			if p == nil {
				t.Fatal("failed to get protocol")
			}
			f, err := os.Open(test.filename)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			var mocks []protocol.Mock
			if err := yaml.NewDecoder(f).Decode(&mocks); err != nil {
				t.Fatal(err)
			}
			iter := protocol.NewMockIterator(mocks)
			defer func() {
				if err := iter.Stop(); err != nil {
					t.Errorf("failed to stop mock iterator: %s", err)
				}
			}()

			// unmarshal config
			cfg, err := p.UnmarshalConfig([]byte(test.config))
			if err != nil {
				t.Fatalf("failed to unmarshal config: %s", err)
			}

			// start server
			srv, err := p.NewServer(iter, logger.NewNopLogger(), cfg)
			if err != nil {
				t.Fatalf("failed to create server: %s", err)
			}
			go func() {
				if err := srv.Start(context.Background()); err != nil {
					t.Errorf("failed to start server: %s", err)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := srv.Wait(ctx); err != nil {
				t.Fatalf("failed to start server: %s", err)
			}

			addr, err := srv.Addr()
			if err != nil {
				t.Errorf("failed to get address: %s", err)
			}
			test.f(t, addr)

			// stop server
			ctx, cancel = context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := srv.Stop(ctx); err != nil {
				t.Fatalf("failed to stop server: %s", err)
			}
		})
	}
}

func sendEchoRequest(st *status.Status, id, msg string) func(t *testing.T, addr string) {
	return func(t *testing.T, addr string) {
		t.Helper()
		c, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("failed to connect server: %s", err)
		}
		client := testpb.NewTestClient(c)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		resp, err := client.Echo(ctx, &testpb.EchoRequest{
			MessageId:   "1",
			MessageBody: "hello",
		})
		if err != nil {
			serr := status.Convert(err)
			if st == nil {
				t.Fatalf("expect status code %s but got %s", codes.OK, serr.Code())
			}
			if got, expect := serr.Code(), st.Code(); got != expect {
				t.Errorf("expect status code %s but got %s", expect, got)
			}
			if got, expect := serr.Message(), st.Message(); !strings.Contains(got, expect) {
				t.Errorf("expect status message %s but got %s", expect, got)
			}
			return
		}
		if st != nil {
			if got, expect := codes.OK, st.Code(); got != expect {
				t.Errorf("expect %s but got %s", expect, got)
			}
		}
		if got, expect := resp.GetMessageId(), id; got != expect {
			t.Errorf("expect %s but got %s", expect, got)
		}
		if got, expect := resp.GetMessageBody(), msg; got != expect {
			t.Errorf("expect %s but got %s", expect, got)
		}
	}
}
