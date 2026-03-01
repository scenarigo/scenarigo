package grpc

import (
	gocontext "context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/ptr"
	"github.com/scenarigo/scenarigo/internal/testutil"
	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestProtoClient(t *testing.T) {
	caCert, serverCert, serverKey := generateCert(t)

	defaultHandler := func(ctx gocontext.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
		return &testpb.EchoResponse{
			MessageId:   req.GetMessageId(),
			MessageBody: req.GetMessageBody(),
		}, nil
	}
	tests := map[string]struct {
		handler           func(gocontext.Context, *testpb.EchoRequest) (*testpb.EchoResponse, error)
		disableReflection bool
		enableTLS         bool
		request           *Request
		expectCode        codes.Code
		expectResponse    *testpb.EchoResponse
		expectError       string
	}{
		"success (proto client)": {
			handler: defaultHandler,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponse: &testpb.EchoResponse{
				MessageId:   "1",
				MessageBody: "hello",
			},
		},
		"success (proto reflection client)": {
			handler: defaultHandler,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponse: &testpb.EchoResponse{
				MessageId:   "1",
				MessageBody: "hello",
			},
		},
		"check User-Agent": {
			handler: func(ctx gocontext.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
				md, ok := metadata.FromIncomingContext(ctx)
				if !ok {
					return nil, status.Error(codes.Internal, "failed to get metadata")
				}
				vs := md.Get("user-agent")
				if len(vs) == 0 {
					return nil, status.Error(codes.Internal, "user-agent not found")
				}
				hs := strings.Split(vs[0], " ")
				if hs[0] != defaultUserAgent || !strings.HasPrefix(hs[1], "grpc-go/") {
					return nil, status.Error(codes.Internal, fmt.Sprintf("expect %q but got %q", vs[0], fmt.Sprintf("%s grpc-go/x.y.z", defaultUserAgent)))
				}
				return &testpb.EchoResponse{
					MessageId:   req.GetMessageId(),
					MessageBody: req.GetMessageBody(),
				}, nil
			},
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponse: &testpb.EchoResponse{
				MessageId:   "1",
				MessageBody: "hello",
			},
		},
		"enable TLS": {
			handler:   defaultHandler,
			enableTLS: true,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						TLS: &TLSOption{
							MinVersion:  "TLS 1.3",
							MaxVersion:  "TLS 1.3",
							Certificate: caCert,
						},
					},
				},
			},
			expectCode: codes.OK,
			expectResponse: &testpb.EchoResponse{
				MessageId:   "1",
				MessageBody: "hello",
			},
		},
		"skip TLS verification": {
			handler:   defaultHandler,
			enableTLS: true,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						TLS: &TLSOption{
							Skip: true,
						},
					},
				},
			},
			expectCode: codes.OK,
			expectResponse: &testpb.EchoResponse{
				MessageId:   "1",
				MessageBody: "hello",
			},
		},

		"server returns error": {
			handler: func(ctx gocontext.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
				return nil, status.New(codes.Unauthenticated, "unauthenticated").Err()
			},
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.Unauthenticated,
		},
		"no TLS certificate": {
			handler:   defaultHandler,
			enableTLS: true,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
				},
			},
			expectCode: codes.Unavailable,
		},
		"unnecessary TLS certificate": {
			handler: defaultHandler,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						TLS: &TLSOption{
							Certificate: caCert,
						},
					},
				},
			},
			expectCode: codes.Unavailable,
		},

		"empty target": {
			handler: defaultHandler,
			request: &Request{
				Target: "",
			},
			expectError: ".target: target must be specified",
		},
		"target is invalid template": {
			handler: defaultHandler,
			request: &Request{
				Target: "{{foo}}",
			},
			expectError: `.target: invalid target: failed to execute: {{foo}}: ".foo" not found`,
		},
		"target must be a string": {
			handler: defaultHandler,
			request: &Request{
				Target: "{{1}}",
			},
			expectError: ".target: target must be string but int64",
		},
		"target is invalid URL": {
			handler: defaultHandler,
			request: &Request{
				Target: "localhost\n",
			},
			expectError: `.target: parse "dns:///localhost\n": net/url: invalid control character in URL`,
		},
		"invalid min TLS version": {
			handler: defaultHandler,
			request: &Request{
				Target: "{{vars.target}}",
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						TLS: &TLSOption{
							MinVersion: "aaa",
						},
					},
				},
			},
			expectError: ".auth.tls.minVersion: invalid minimum TLS version aaa",
		},
		"invalid max TLS version": {
			handler: defaultHandler,
			request: &Request{
				Target: "{{vars.target}}",
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						TLS: &TLSOption{
							MaxVersion: "aaa",
						},
					},
				},
			},
			expectError: ".auth.tls.maxVersion: invalid maximum TLS version aaa",
		},
		"TLS certificate not found": {
			handler: defaultHandler,
			request: &Request{
				Target: "{{vars.target}}",
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						TLS: &TLSOption{
							Certificate: "foo.cert",
						},
					},
				},
			},
			expectError: ".auth.tls.certificate: failed to read certificate: open foo.cert: no such file or directory",
		},
		"proto file not found": {
			handler: defaultHandler,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"foo.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectError: ".options.proto: failed to compile: open foo.proto: no such file or directory",
		},
		"reflection service is not implemented": {
			handler:           defaultHandler,
			disableReflection: true,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{},
				Options: &RequestOptions{
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectError: "doesn't implement gRPC reflection service: rpc error: code = Unimplemented desc = unknown service grpc.reflection.v1alpha.ServerReflection",
		},
		"service not found in proto": {
			handler: defaultHandler,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: "unknown",
				Method:  "Echo",
				Message: yaml.MapSlice{},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectError: `.service: service "unknown" not found`,
		},
		"service not found in reflection": {
			handler: defaultHandler,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: "unknown",
				Method:  "Echo",
				Message: yaml.MapSlice{},
				Options: &RequestOptions{
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectError: ".service: Service not found: unknown",
		},
		"method not found": {
			handler: defaultHandler,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Unknown",
				Message: yaml.MapSlice{},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectError: `.method: method "Unknown" not found`,
		},
		"unary with messages is invalid": {
			handler: defaultHandler,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Messages: []any{
					yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "1"},
						yaml.MapItem{Key: "messageBody", Value: "hello"},
					},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectError: ".messages: messages can only be used with streaming methods",
		},
		"invalid request message": {
			handler: defaultHandler,
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "Echo",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: 1},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectError: ".message: failed to build request message",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := testutil.TestGRPCServerFunc(test.handler)
			var opts []testutil.TestGRPCServerOption
			if !test.disableReflection {
				opts = append(opts, testutil.EnableReflection())
			}
			if test.enableTLS {
				opts = append(opts, testutil.EnableTLS(serverCert, serverKey))
			}
			target := testutil.StartTestGRPCServer(t, srv, opts...)
			t.Cleanup(func() { _ = connPool.closeConnection(target) })
			ctx := context.FromT(t).WithVars(map[string]any{
				"target": target,
			})

			_, result, err := test.request.Invoke(ctx)
			if test.expectError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
			} else {
				if got := err.Error(); !strings.Contains(got, test.expectError) {
					t.Fatalf("expected error is %q but got %q", test.expectError, got)
				}
				return
			}

			resp, ok := result.(*response)
			if !ok {
				t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
			}
			serr := resp.Status
			if serr == nil {
				t.Fatal("no error")
			}
			if serr.Code() != test.expectCode {
				t.Fatalf("expected code is %s but got %s: %s", test.expectCode, serr.Code(), serr.Err())
			}
			if test.expectCode == codes.OK {
				if diff := cmp.Diff(test.expectResponse, resp.Message, protocmp.Transform()); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
			}
		})
	}
}

func TestProtoClientServerStreaming(t *testing.T) {
	defaultHandler := func(ctx gocontext.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
		return &testpb.EchoResponse{
			MessageId:   req.GetMessageId(),
			MessageBody: req.GetMessageBody(),
		}, nil
	}
	tests := map[string]struct {
		request         *Request
		expectCode      codes.Code
		expectResponses []*testpb.EchoResponse
		expectError     string
	}{
		"server streaming with proto files": {
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "ServerStreamEcho",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponses: []*testpb.EchoResponse{
				{MessageId: "1", MessageBody: "hello-0"},
				{MessageId: "1", MessageBody: "hello-1"},
				{MessageId: "1", MessageBody: "hello-2"},
			},
		},
		"server streaming with reflection": {
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "ServerStreamEcho",
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
				Options: &RequestOptions{
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponses: []*testpb.EchoResponse{
				{MessageId: "1", MessageBody: "hello-0"},
				{MessageId: "1", MessageBody: "hello-1"},
				{MessageId: "1", MessageBody: "hello-2"},
			},
		},
		"server streaming with messages is invalid": {
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "ServerStreamEcho",
				Messages: []any{
					yaml.MapSlice{yaml.MapItem{Key: "messageBody", Value: "hello"}},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectError: ".messages: messages cannot be used for server streaming request",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := testutil.TestGRPCServerFunc(defaultHandler)
			target := testutil.StartTestGRPCServer(t, srv, testutil.EnableReflection())
			t.Cleanup(func() { _ = connPool.closeConnection(target) })
			ctx := context.FromT(t).WithVars(map[string]any{
				"target": target,
			})

			_, result, err := test.request.Invoke(ctx)
			if test.expectError != "" {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if got := err.Error(); !strings.Contains(got, test.expectError) {
					t.Fatalf("expected error %q but got %q", test.expectError, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			resp, ok := result.(*response)
			if !ok {
				t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
			}
			if resp.Status.Code() != test.expectCode {
				t.Fatalf("expected code is %s but got %s", test.expectCode, resp.Status.Code())
			}
			if len(test.expectResponses) > 0 {
				if len(resp.Messages) != len(test.expectResponses) {
					t.Fatalf("expected %d messages but got %d", len(test.expectResponses), len(resp.Messages))
				}
				for i, expected := range test.expectResponses {
					if diff := cmp.Diff(expected, resp.Messages[i], protocmp.Transform()); diff != "" {
						t.Errorf("messages[%d] differs: (-want +got)\n%s", i, diff)
					}
				}
			}
		})
	}
}

func TestProtoClientClientStreaming(t *testing.T) {
	defaultHandler := func(ctx gocontext.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
		return &testpb.EchoResponse{
			MessageId:   req.GetMessageId(),
			MessageBody: req.GetMessageBody(),
		}, nil
	}
	tests := map[string]struct {
		request        *Request
		expectCode     codes.Code
		expectResponse *testpb.EchoResponse
		expectError    string
	}{
		"client streaming with proto files": {
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "ClientStreamEcho",
				Messages: []any{
					yaml.MapSlice{yaml.MapItem{Key: "messageBody", Value: "hello"}},
					yaml.MapSlice{yaml.MapItem{Key: "messageBody", Value: "world"}},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponse: &testpb.EchoResponse{
				MessageId:   "aggregated",
				MessageBody: "hello,world",
			},
		},
		"client streaming with reflection": {
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "ClientStreamEcho",
				Messages: []any{
					yaml.MapSlice{yaml.MapItem{Key: "messageBody", Value: "hello"}},
					yaml.MapSlice{yaml.MapItem{Key: "messageBody", Value: "world"}},
				},
				Options: &RequestOptions{
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponse: &testpb.EchoResponse{
				MessageId:   "aggregated",
				MessageBody: "hello,world",
			},
		},
		"client streaming with message is invalid": {
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "ClientStreamEcho",
				Message: yaml.MapSlice{yaml.MapItem{Key: "messageBody", Value: "hello"}},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectError: ".message: message cannot be used for client streaming request",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := testutil.TestGRPCServerFunc(defaultHandler)
			target := testutil.StartTestGRPCServer(t, srv, testutil.EnableReflection())
			t.Cleanup(func() { _ = connPool.closeConnection(target) })
			ctx := context.FromT(t).WithVars(map[string]any{
				"target": target,
			})

			_, result, err := test.request.Invoke(ctx)
			if test.expectError != "" {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if got := err.Error(); !strings.Contains(got, test.expectError) {
					t.Fatalf("expected error %q but got %q", test.expectError, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			resp, ok := result.(*response)
			if !ok {
				t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
			}
			if resp.Status.Code() != test.expectCode {
				t.Fatalf("expected code is %s but got %s", test.expectCode, resp.Status.Code())
			}
			if test.expectResponse != nil {
				if diff := cmp.Diff(test.expectResponse, resp.Message, protocmp.Transform()); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
			}
		})
	}
}

func TestProtoClientBidiStreaming(t *testing.T) {
	defaultHandler := func(ctx gocontext.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
		return &testpb.EchoResponse{
			MessageId:   req.GetMessageId(),
			MessageBody: req.GetMessageBody(),
		}, nil
	}
	tests := map[string]struct {
		request         *Request
		expectCode      codes.Code
		expectResponses []*testpb.EchoResponse
		expectError     string
	}{
		"bidi streaming with proto files": {
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "BidiStreamEcho",
				Messages: []any{
					yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "1"},
						yaml.MapItem{Key: "messageBody", Value: "hello"},
					},
					yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "2"},
						yaml.MapItem{Key: "messageBody", Value: "world"},
					},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponses: []*testpb.EchoResponse{
				{MessageId: "1", MessageBody: "re: hello"},
				{MessageId: "2", MessageBody: "re: world"},
			},
		},
		"bidi streaming with reflection": {
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "BidiStreamEcho",
				Messages: []any{
					yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "1"},
						yaml.MapItem{Key: "messageBody", Value: "hello"},
					},
					yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "2"},
						yaml.MapItem{Key: "messageBody", Value: "world"},
					},
				},
				Options: &RequestOptions{
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponses: []*testpb.EchoResponse{
				{MessageId: "1", MessageBody: "re: hello"},
				{MessageId: "2", MessageBody: "re: world"},
			},
		},
		"bidi streaming with response reference": {
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "BidiStreamEcho",
				Messages: []any{
					yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "1"},
						yaml.MapItem{Key: "messageBody", Value: "hello"},
					},
					yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "2"},
						yaml.MapItem{Key: "messageBody", Value: "{{response.messages[0].messageBody}} world"},
					},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponses: []*testpb.EchoResponse{
				{MessageId: "1", MessageBody: "re: hello"},
				{MessageId: "2", MessageBody: "re: re: hello world"},
			},
		},
		"bidi streaming with request reference": {
			request: &Request{
				Target:  "{{vars.target}}",
				Service: testpb.Test_ServiceDesc.ServiceName,
				Method:  "BidiStreamEcho",
				Messages: []any{
					yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "1"},
						yaml.MapItem{Key: "messageBody", Value: "hello"},
					},
					yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "2"},
						yaml.MapItem{Key: "messageBody", Value: "{{request.messages[0].messageBody}} world"},
					},
				},
				Options: &RequestOptions{
					Proto: &ProtoOption{
						Files: []string{
							"../../testdata/proto/test/test.proto",
						},
					},
					Auth: &AuthOption{
						Insecure: ptr.To(true),
					},
				},
			},
			expectCode: codes.OK,
			expectResponses: []*testpb.EchoResponse{
				{MessageId: "1", MessageBody: "re: hello"},
				{MessageId: "2", MessageBody: "re: hello world"},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := testutil.TestGRPCServerFunc(defaultHandler)
			target := testutil.StartTestGRPCServer(t, srv, testutil.EnableReflection())
			t.Cleanup(func() { _ = connPool.closeConnection(target) })
			ctx := context.FromT(t).WithVars(map[string]any{
				"target": target,
			})

			_, result, err := test.request.Invoke(ctx)
			if test.expectError != "" {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if got := err.Error(); !strings.Contains(got, test.expectError) {
					t.Fatalf("expected error %q but got %q", test.expectError, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			resp, ok := result.(*response)
			if !ok {
				t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
			}
			if resp.Status.Code() != test.expectCode {
				t.Fatalf("expected code is %s but got %s", test.expectCode, resp.Status.Code())
			}
			if len(test.expectResponses) > 0 {
				if len(resp.Messages) != len(test.expectResponses) {
					t.Fatalf("expected %d messages but got %d", len(test.expectResponses), len(resp.Messages))
				}
				for i, expected := range test.expectResponses {
					if diff := cmp.Diff(expected, resp.Messages[i], protocmp.Transform()); diff != "" {
						t.Errorf("messages[%d] differs: (-want +got)\n%s", i, diff)
					}
				}
			}
		})
	}
}

func generateCert(t *testing.T) (string, string, string) {
	t.Helper()
	tmp := t.TempDir()
	now := time.Now()
	validityPeriod := time.Hour

	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             now,
		NotAfter:              now.Add(validityPeriod),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %s", err)
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, caPub, caPriv)
	if err != nil {
		t.Fatalf("failed to create certificate: %s", err)
	}
	caPEM, err := os.Create(filepath.Join(tmp, "ca.crt"))
	if err != nil {
		t.Fatalf("failed to create ca.crt: %s", err)
	}
	defer caPEM.Close()
	if err := pem.Encode(caPEM, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes}); err != nil {
		t.Fatalf("failed to encode PEM: %s", err)
	}

	cert := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             now,
		NotAfter:              now.Add(validityPeriod),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
	}
	certPub, certPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %s", err)
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, certPub, caPriv)
	if err != nil {
		t.Fatalf("failed to create certificate: %s", err)
	}
	certPEM, err := os.Create(filepath.Join(tmp, "server.crt"))
	if err != nil {
		t.Fatalf("failed to create server.crt: %s", err)
	}
	if err := pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		t.Fatalf("failed to encode PEM: %s", err)
	}
	certKeyPEM, err := os.OpenFile(filepath.Join(tmp, "server.key"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("failed to create server.key: %s", err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(certPriv)
	if err != nil {
		t.Fatalf("unable to marshal private key: %s", err)
	}
	if err := pem.Encode(certKeyPEM, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		t.Fatalf("failed to encode PEM: %s", err)
	}

	return caPEM.Name(), certPEM.Name(), certKeyPEM.Name()
}
