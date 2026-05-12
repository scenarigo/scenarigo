package plugin

import (
	gocontext "context"
	"errors"
	"io"
	"reflect"
	"testing"

	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestValidateGRPCMethod(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		method := reflect.ValueOf(testpb.NewTestClient(nil)).MethodByName("Echo")
		if err := ValidateGRPCMethod(method); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		tests := map[string]struct {
			method reflect.Value
		}{
			"invalid": {
				method: reflect.Value{},
			},
			"must be func": {
				method: reflect.ValueOf(struct{}{}),
			},
			"nil": {
				method: reflect.ValueOf((func())(nil)),
			},
			"number of arguments must be 3": {
				method: reflect.ValueOf(func() (proto.Message, error) {
					return nil, nil //nolint:nilnil
				}),
			},
			"first argument must be context.Context": {
				method: reflect.ValueOf(func(ctx struct{}, in proto.Message, opts ...grpc.CallOption) (proto.Message, error) {
					return nil, nil //nolint:nilnil
				}),
			},
			"second argument must be proto.Message": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in struct{}, opts ...grpc.CallOption) (proto.Message, error) {
					return nil, nil //nolint:nilnil
				}),
			},
			"third argument must be []grpc.CallOption": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in proto.Message, opts ...struct{}) (proto.Message, error) {
					return nil, nil //nolint:nilnil
				}),
			},
			"number of return values must be 2": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in proto.Message, opts ...grpc.CallOption) {
				}),
			},
			"first return value must be proto.Message": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in proto.Message, opts ...grpc.CallOption) (*struct{}, error) {
					return nil, nil //nolint:nilnil
				}),
			},
			"second return value must be error": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in proto.Message, opts ...grpc.CallOption) (proto.Message, *struct{}) {
					return nil, nil
				}),
			},
		}
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				if err := ValidateGRPCMethod(tc.method); err == nil {
					t.Fatal("no error")
				}
			})
		}
	})
}

func TestGRPCInvoke(t *testing.T) {
	ctx := gocontext.Background()

	t.Run("successful invocation", func(t *testing.T) {
		// Create a mock gRPC method that returns a successful response
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req proto.Message, opts ...grpc.CallOption) (proto.Message, error) {
			// Verify the request is the expected type
			echoReq, ok := req.(*testpb.EchoRequest)
			if !ok {
				t.Errorf("Expected *testpb.EchoRequest, got %T", req)
				return nil, status.Errorf(codes.InvalidArgument, "invalid request type")
			}

			// Return a response
			return &testpb.EchoResponse{
				MessageId:   echoReq.GetMessageId(),
				MessageBody: echoReq.GetMessageBody(),
				ReceivedAt:  1234567890,
				UserType:    testpb.UserType_CUSTOMER,
				State:       testpb.State_ACTIVE,
			}, nil
		})

		reqMsg := &testpb.EchoRequest{
			MessageId:   "test-123",
			MessageBody: "Hello World",
		}

		respMsg, sts, err := GRPCInvoke(ctx, mockMethod, reqMsg)
		if err != nil {
			t.Fatalf("GRPCInvoke failed: %v", err)
		}

		if sts != nil {
			t.Fatalf("Expected nil status, got %v", sts)
		}

		if respMsg == nil {
			t.Fatal("Expected response message, got nil")
		}

		echoResp, ok := respMsg.(*testpb.EchoResponse)
		if !ok {
			t.Fatalf("Expected *testpb.EchoResponse, got %T", respMsg)
		}

		if echoResp.GetMessageId() != "test-123" {
			t.Errorf("MessageId mismatch: got %s, want test-123", echoResp.GetMessageId())
		}

		if echoResp.GetMessageBody() != "Hello World" {
			t.Errorf("MessageBody mismatch: got %s, want Hello World", echoResp.GetMessageBody())
		}
	})

	t.Run("gRPC error status", func(t *testing.T) {
		// Create a mock gRPC method that returns a gRPC status error
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req proto.Message, opts ...grpc.CallOption) (proto.Message, error) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid request")
		})

		reqMsg := &testpb.EchoRequest{
			MessageId:   "test-error",
			MessageBody: "Error Test",
		}

		respMsg, sts, err := GRPCInvoke(ctx, mockMethod, reqMsg)
		if err != nil {
			t.Fatalf("GRPCInvoke failed: %v", err)
		}

		if respMsg != nil {
			t.Fatalf("Expected nil response, got %v", respMsg)
		}

		if sts == nil {
			t.Fatal("Expected status, got nil")
		}

		if sts.Code() != codes.InvalidArgument {
			t.Errorf("Status code mismatch: got %v, want %v", sts.Code(), codes.InvalidArgument)
		}

		if sts.Message() != "invalid request" {
			t.Errorf("Status message mismatch: got %s, want invalid request", sts.Message())
		}
	})

	t.Run("non-gRPC error", func(t *testing.T) {
		// Create a mock gRPC method that returns a non-gRPC error
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req proto.Message, opts ...grpc.CallOption) (proto.Message, error) {
			return nil, errors.New("some internal error")
		})

		reqMsg := &testpb.EchoRequest{
			MessageId:   "test-internal-error",
			MessageBody: "Internal Error Test",
		}

		_, _, err := GRPCInvoke(ctx, mockMethod, reqMsg)
		if err == nil {
			t.Fatal("Expected error for non-gRPC error, got nil")
		}

		expectedErrMsg := `expected gRPC status error but got *errors.errorString: "some internal error"`
		if err.Error() != expectedErrMsg {
			t.Errorf("Error message mismatch:\ngot:  %s\nwant: %s", err.Error(), expectedErrMsg)
		}
	})

	t.Run("nil response and nil error", func(t *testing.T) {
		// Create a mock gRPC method that returns nil response and nil error
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req proto.Message, opts ...grpc.CallOption) (proto.Message, error) {
			return nil, nil //nolint:nilnil
		})

		reqMsg := &testpb.EchoRequest{
			MessageId:   "test-nil",
			MessageBody: "Nil Test",
		}

		respMsg, sts, err := GRPCInvoke(ctx, mockMethod, reqMsg)
		if err != nil {
			t.Fatalf("GRPCInvoke failed: %v", err)
		}

		if respMsg != nil {
			t.Fatalf("Expected nil response, got %v", respMsg)
		}

		if sts != nil {
			t.Fatalf("Expected nil status, got %v", sts)
		}
	})

	t.Run("invalid method - wrong number of return values", func(t *testing.T) {
		// Create a mock method with wrong number of return values
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req proto.Message, opts ...grpc.CallOption) proto.Message {
			return &testpb.EchoResponse{}
		})

		reqMsg := &testpb.EchoRequest{
			MessageId:   "test-invalid",
			MessageBody: "Invalid Test",
		}

		_, _, err := GRPCInvoke(ctx, mockMethod, reqMsg)
		if err == nil {
			t.Fatal("Expected error for invalid method, got nil")
		}

		expectedErrMsg := "expected return value length of method call is 2 but 1"
		if err.Error() != expectedErrMsg {
			t.Errorf("Error message mismatch:\ngot:  %s\nwant: %s", err.Error(), expectedErrMsg)
		}
	})

	t.Run("invalid first return value", func(t *testing.T) {
		// Create a mock method that returns invalid first return value (pointer type to avoid IsNil panic)
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req proto.Message, opts ...grpc.CallOption) (*string, error) {
			s := "not a proto message"
			return &s, nil
		})

		reqMsg := &testpb.EchoRequest{
			MessageId:   "test-invalid-return",
			MessageBody: "Invalid Return Test",
		}

		_, _, err := GRPCInvoke(ctx, mockMethod, reqMsg)
		if err == nil {
			t.Fatal("Expected error for invalid first return value, got nil")
		}

		expectedErrMsg := "expected first return value is proto.Message but *string"
		if err.Error() != expectedErrMsg {
			t.Errorf("Error message mismatch:\ngot:  %s\nwant: %s", err.Error(), expectedErrMsg)
		}
	})

	t.Run("invalid second return value", func(t *testing.T) {
		// Create a mock method that returns invalid second return value (pointer type to avoid IsNil panic)
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req proto.Message, opts ...grpc.CallOption) (proto.Message, *string) {
			s := "not an error"
			return &testpb.EchoResponse{}, &s
		})

		reqMsg := &testpb.EchoRequest{
			MessageId:   "test-invalid-error",
			MessageBody: "Invalid Error Test",
		}

		_, _, err := GRPCInvoke(ctx, mockMethod, reqMsg)
		if err == nil {
			t.Fatal("Expected error for invalid second return value, got nil")
		}

		expectedErrMsg := "expected second return value is error but *string"
		if err.Error() != expectedErrMsg {
			t.Errorf("Error message mismatch:\ngot:  %s\nwant: %s", err.Error(), expectedErrMsg)
		}
	})

	t.Run("with call options", func(t *testing.T) {
		// Create a mock gRPC method that accepts call options
		var receivedOpts []grpc.CallOption
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req proto.Message, opts ...grpc.CallOption) (proto.Message, error) {
			receivedOpts = opts
			return &testpb.EchoResponse{
				MessageId:   "test-opts",
				MessageBody: "Options Test",
				ReceivedAt:  1234567890,
			}, nil
		})

		reqMsg := &testpb.EchoRequest{
			MessageId:   "test-opts",
			MessageBody: "Options Test",
		}

		// Create some call options
		opts := []grpc.CallOption{
			grpc.MaxCallRecvMsgSize(1024),
			grpc.MaxCallSendMsgSize(1024),
		}

		respMsg, sts, err := GRPCInvoke(ctx, mockMethod, reqMsg, opts...)
		if err != nil {
			t.Fatalf("GRPCInvoke failed: %v", err)
		}

		if sts != nil {
			t.Fatalf("Expected nil status, got %v", sts)
		}

		if respMsg == nil {
			t.Fatal("Expected response message, got nil")
		}

		// Verify that call options were passed correctly
		if len(receivedOpts) != len(opts) {
			t.Errorf("Call options length mismatch: got %d, want %d", len(receivedOpts), len(opts))
		}
	})
}

func TestDetectGRPCMethodType(t *testing.T) {
	client := testpb.NewTestClient(nil)
	v := reflect.ValueOf(client)

	tests := map[string]struct {
		methodName string
		expected   GRPCMethodType
	}{
		"unary": {
			methodName: "Echo",
			expected:   GRPCMethodUnary,
		},
		"server stream": {
			methodName: "ServerStreamEcho",
			expected:   GRPCMethodServerStream,
		},
		"client stream": {
			methodName: "ClientStreamEcho",
			expected:   GRPCMethodClientStream,
		},
		"bidi stream": {
			methodName: "BidiStreamEcho",
			expected:   GRPCMethodBidiStream,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			method := v.MethodByName(tc.methodName)
			got := DetectGRPCMethodType(method)
			if got != tc.expected {
				t.Errorf("DetectGRPCMethodType() = %d, want %d", got, tc.expected)
			}
		})
	}

	t.Run("fallback for unusual arg count", func(t *testing.T) {
		// A function with 1 arg should fall through to default Unary
		method := reflect.ValueOf(func(ctx gocontext.Context) {})
		got := DetectGRPCMethodType(method)
		if got != GRPCMethodUnary {
			t.Errorf("DetectGRPCMethodType() = %d, want %d", got, GRPCMethodUnary)
		}
	})
}

func TestValidateGRPCStreamingMethod(t *testing.T) {
	client := testpb.NewTestClient(nil)
	v := reflect.ValueOf(client)

	t.Run("valid server stream", func(t *testing.T) {
		method := v.MethodByName("ServerStreamEcho")
		if err := ValidateGRPCStreamingMethod(method, GRPCMethodServerStream); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
	})
	t.Run("valid client stream", func(t *testing.T) {
		method := v.MethodByName("ClientStreamEcho")
		if err := ValidateGRPCStreamingMethod(method, GRPCMethodClientStream); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
	})
	t.Run("valid bidi stream", func(t *testing.T) {
		method := v.MethodByName("BidiStreamEcho")
		if err := ValidateGRPCStreamingMethod(method, GRPCMethodBidiStream); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		tests := map[string]struct {
			method     reflect.Value
			methodType GRPCMethodType
		}{
			"invalid value": {
				method:     reflect.Value{},
				methodType: GRPCMethodServerStream,
			},
			"not function": {
				method:     reflect.ValueOf(struct{}{}),
				methodType: GRPCMethodServerStream,
			},
			"nil function": {
				method:     reflect.ValueOf((func())(nil)),
				methodType: GRPCMethodServerStream,
			},
			"server stream wrong arg count": {
				method: reflect.ValueOf(func(ctx gocontext.Context, opts ...grpc.CallOption) (testpb.Test_ServerStreamEchoClient, error) {
					return nil, nil //nolint:nilnil
				}),
				methodType: GRPCMethodServerStream,
			},
			"server stream first arg not context": {
				method: reflect.ValueOf(func(ctx struct{}, in *testpb.EchoRequest, opts ...grpc.CallOption) (testpb.Test_ServerStreamEchoClient, error) {
					return nil, nil //nolint:nilnil
				}),
				methodType: GRPCMethodServerStream,
			},
			"server stream second arg not proto.Message": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in struct{}, opts ...grpc.CallOption) (testpb.Test_ServerStreamEchoClient, error) {
					return nil, nil //nolint:nilnil
				}),
				methodType: GRPCMethodServerStream,
			},
			"server stream third arg not CallOption": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in *testpb.EchoRequest, opts ...struct{}) (testpb.Test_ServerStreamEchoClient, error) {
					return nil, nil //nolint:nilnil
				}),
				methodType: GRPCMethodServerStream,
			},
			"client stream wrong arg count": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in *testpb.EchoRequest, opts ...grpc.CallOption) (testpb.Test_ClientStreamEchoClient, error) {
					return nil, nil //nolint:nilnil
				}),
				methodType: GRPCMethodClientStream,
			},
			"client stream first arg not context": {
				method: reflect.ValueOf(func(ctx struct{}, opts ...grpc.CallOption) (testpb.Test_ClientStreamEchoClient, error) {
					return nil, nil //nolint:nilnil
				}),
				methodType: GRPCMethodClientStream,
			},
			"client stream second arg not CallOption": {
				method: reflect.ValueOf(func(ctx gocontext.Context, opts ...struct{}) (testpb.Test_ClientStreamEchoClient, error) {
					return nil, nil //nolint:nilnil
				}),
				methodType: GRPCMethodClientStream,
			},
			"wrong return count": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in *testpb.EchoRequest, opts ...grpc.CallOption) testpb.Test_ServerStreamEchoClient {
					return nil
				}),
				methodType: GRPCMethodServerStream,
			},
			"first return not ClientStream": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in *testpb.EchoRequest, opts ...grpc.CallOption) (*testpb.EchoResponse, error) {
					return nil, nil //nolint:nilnil
				}),
				methodType: GRPCMethodServerStream,
			},
			"second return not error": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in *testpb.EchoRequest, opts ...grpc.CallOption) (testpb.Test_ServerStreamEchoClient, *struct{}) {
					return nil, nil
				}),
				methodType: GRPCMethodServerStream,
			},
			"unexpected method type": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in *testpb.EchoRequest, opts ...grpc.CallOption) (testpb.Test_ServerStreamEchoClient, error) {
					return nil, nil //nolint:nilnil
				}),
				methodType: GRPCMethodUnary,
			},
		}
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				if err := ValidateGRPCStreamingMethod(tc.method, tc.methodType); err == nil {
					t.Fatal("expected error but got nil")
				}
			})
		}
	})
}

// mockStream is a test helper for stream operation tests.
type mockStream struct {
	sendFunc         func(proto.Message) error
	recvFunc         func() (*testpb.EchoResponse, error)
	closeAndRecvFunc func() (*testpb.EchoResponse, error)
	closeSendFunc    func() error
}

func (m *mockStream) Send(msg *testpb.EchoRequest) error {
	if m.sendFunc != nil {
		return m.sendFunc(msg)
	}
	return nil
}

func (m *mockStream) Recv() (*testpb.EchoResponse, error) {
	if m.recvFunc != nil {
		return m.recvFunc()
	}
	return nil, io.EOF
}

func (m *mockStream) CloseAndRecv() (*testpb.EchoResponse, error) {
	if m.closeAndRecvFunc != nil {
		return m.closeAndRecvFunc()
	}
	return &testpb.EchoResponse{}, nil
}

func (m *mockStream) CloseSend() error {
	if m.closeSendFunc != nil {
		return m.closeSendFunc()
	}
	return nil
}

// mockClientStream implements grpc.ClientStream for GRPCStreamAsClientStream tests.
type mockClientStream struct{}

func (m *mockClientStream) Header() (metadata.MD, error) { return nil, nil } //nolint:nilnil
func (m *mockClientStream) Trailer() metadata.MD         { return nil }
func (m *mockClientStream) CloseSend() error             { return nil }
func (m *mockClientStream) Context() gocontext.Context   { return gocontext.Background() }
func (m *mockClientStream) SendMsg(any) error            { return nil }
func (m *mockClientStream) RecvMsg(any) error            { return nil }

func TestGRPCInvokeServerStream(t *testing.T) {
	ctx := gocontext.Background()

	t.Run("successful invocation", func(t *testing.T) {
		stream := &mockStream{}
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req *testpb.EchoRequest, opts ...grpc.CallOption) (*mockStream, error) {
			return stream, nil
		})
		reqMsg := &testpb.EchoRequest{MessageId: "test"}

		result, err := GRPCInvokeServerStream(ctx, mockMethod, reqMsg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsNil() {
			t.Fatal("expected non-nil stream")
		}
	})

	t.Run("method returns error", func(t *testing.T) {
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req *testpb.EchoRequest, opts ...grpc.CallOption) (*mockStream, error) {
			return nil, errors.New("connection failed")
		})
		reqMsg := &testpb.EchoRequest{MessageId: "test"}

		_, err := GRPCInvokeServerStream(ctx, mockMethod, reqMsg)
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "connection failed" {
			t.Errorf("error mismatch: got %q, want %q", err.Error(), "connection failed")
		}
	})

	t.Run("with call options", func(t *testing.T) {
		var receivedOpts []grpc.CallOption
		stream := &mockStream{}
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, req *testpb.EchoRequest, opts ...grpc.CallOption) (*mockStream, error) {
			receivedOpts = opts
			return stream, nil
		})
		reqMsg := &testpb.EchoRequest{MessageId: "test"}
		opts := []grpc.CallOption{grpc.MaxCallRecvMsgSize(1024)}

		_, err := GRPCInvokeServerStream(ctx, mockMethod, reqMsg, opts...)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(receivedOpts) != 1 {
			t.Errorf("expected 1 call option, got %d", len(receivedOpts))
		}
	})
}

func TestGRPCInvokeStream(t *testing.T) {
	ctx := gocontext.Background()

	t.Run("successful invocation", func(t *testing.T) {
		stream := &mockStream{}
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, opts ...grpc.CallOption) (*mockStream, error) {
			return stream, nil
		})

		result, err := GRPCInvokeStream(ctx, mockMethod)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsNil() {
			t.Fatal("expected non-nil stream")
		}
	})

	t.Run("method returns error", func(t *testing.T) {
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, opts ...grpc.CallOption) (*mockStream, error) {
			return nil, errors.New("stream failed")
		})

		_, err := GRPCInvokeStream(ctx, mockMethod)
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "stream failed" {
			t.Errorf("error mismatch: got %q, want %q", err.Error(), "stream failed")
		}
	})

	t.Run("with call options", func(t *testing.T) {
		var receivedOpts []grpc.CallOption
		stream := &mockStream{}
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, opts ...grpc.CallOption) (*mockStream, error) {
			receivedOpts = opts
			return stream, nil
		})
		opts := []grpc.CallOption{grpc.MaxCallRecvMsgSize(1024)}

		_, err := GRPCInvokeStream(ctx, mockMethod, opts...)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(receivedOpts) != 1 {
			t.Errorf("expected 1 call option, got %d", len(receivedOpts))
		}
	})
}

func TestGRPCStreamSend(t *testing.T) {
	t.Run("successful send", func(t *testing.T) {
		var sentMsg proto.Message
		stream := &mockStream{
			sendFunc: func(msg proto.Message) error {
				sentMsg = msg
				return nil
			},
		}
		msg := &testpb.EchoRequest{MessageId: "hello"}

		err := GRPCStreamSend(reflect.ValueOf(stream), msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sentMsg == nil {
			t.Fatal("expected message to be sent")
		}
	})

	t.Run("send returns error", func(t *testing.T) {
		stream := &mockStream{
			sendFunc: func(msg proto.Message) error {
				return errors.New("send failed")
			},
		}
		msg := &testpb.EchoRequest{MessageId: "hello"}

		err := GRPCStreamSend(reflect.ValueOf(stream), msg)
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "send failed" {
			t.Errorf("error mismatch: got %q, want %q", err.Error(), "send failed")
		}
	})

	t.Run("no Send method", func(t *testing.T) {
		stream := struct{}{}
		err := GRPCStreamSend(reflect.ValueOf(&stream), &testpb.EchoRequest{})
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "stream does not have Send method" {
			t.Errorf("error mismatch: got %q", err.Error())
		}
	})
}

func TestGRPCStreamRecv(t *testing.T) {
	t.Run("successful recv", func(t *testing.T) {
		stream := &mockStream{
			recvFunc: func() (*testpb.EchoResponse, error) {
				return &testpb.EchoResponse{MessageId: "resp"}, nil
			},
		}

		msg, err := GRPCStreamRecv(reflect.ValueOf(stream))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp, ok := msg.(*testpb.EchoResponse)
		if !ok {
			t.Fatalf("expected *testpb.EchoResponse, got %T", msg)
		}
		if resp.GetMessageId() != "resp" {
			t.Errorf("MessageId mismatch: got %q, want %q", resp.GetMessageId(), "resp")
		}
	})

	t.Run("recv returns error", func(t *testing.T) {
		stream := &mockStream{
			recvFunc: func() (*testpb.EchoResponse, error) {
				return nil, errors.New("recv failed")
			},
		}

		_, err := GRPCStreamRecv(reflect.ValueOf(stream))
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "recv failed" {
			t.Errorf("error mismatch: got %q, want %q", err.Error(), "recv failed")
		}
	})

	t.Run("recv returns nil message", func(t *testing.T) {
		stream := &mockStream{
			recvFunc: func() (*testpb.EchoResponse, error) {
				return nil, nil //nolint:nilnil
			},
		}

		_, err := GRPCStreamRecv(reflect.ValueOf(stream))
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if !errors.Is(err, io.EOF) {
			t.Errorf("expected io.EOF, got %v", err)
		}
	})

	t.Run("no Recv method", func(t *testing.T) {
		stream := struct{}{}
		_, err := GRPCStreamRecv(reflect.ValueOf(&stream))
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "stream does not have Recv method" {
			t.Errorf("error mismatch: got %q", err.Error())
		}
	})
}

func TestGRPCStreamCloseAndRecv(t *testing.T) {
	t.Run("successful close and recv", func(t *testing.T) {
		stream := &mockStream{
			closeAndRecvFunc: func() (*testpb.EchoResponse, error) {
				return &testpb.EchoResponse{MessageId: "final"}, nil
			},
		}

		msg, err := GRPCStreamCloseAndRecv(reflect.ValueOf(stream))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp, ok := msg.(*testpb.EchoResponse)
		if !ok {
			t.Fatalf("expected *testpb.EchoResponse, got %T", msg)
		}
		if resp.GetMessageId() != "final" {
			t.Errorf("MessageId mismatch: got %q, want %q", resp.GetMessageId(), "final")
		}
	})

	t.Run("close and recv returns error", func(t *testing.T) {
		stream := &mockStream{
			closeAndRecvFunc: func() (*testpb.EchoResponse, error) {
				return nil, errors.New("close failed")
			},
		}

		_, err := GRPCStreamCloseAndRecv(reflect.ValueOf(stream))
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "close failed" {
			t.Errorf("error mismatch: got %q, want %q", err.Error(), "close failed")
		}
	})

	t.Run("close and recv returns nil response", func(t *testing.T) {
		stream := &mockStream{
			closeAndRecvFunc: func() (*testpb.EchoResponse, error) {
				return nil, nil //nolint:nilnil
			},
		}

		_, err := GRPCStreamCloseAndRecv(reflect.ValueOf(stream))
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "CloseAndRecv returned nil response" {
			t.Errorf("error mismatch: got %q", err.Error())
		}
	})

	t.Run("no CloseAndRecv method", func(t *testing.T) {
		stream := struct{}{}
		_, err := GRPCStreamCloseAndRecv(reflect.ValueOf(&stream))
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "stream does not have CloseAndRecv method" {
			t.Errorf("error mismatch: got %q", err.Error())
		}
	})
}

func TestGRPCStreamCloseSend(t *testing.T) {
	t.Run("successful close send", func(t *testing.T) {
		stream := &mockStream{
			closeSendFunc: func() error {
				return nil
			},
		}

		err := GRPCStreamCloseSend(reflect.ValueOf(stream))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("close send returns error", func(t *testing.T) {
		stream := &mockStream{
			closeSendFunc: func() error {
				return errors.New("close send failed")
			},
		}

		err := GRPCStreamCloseSend(reflect.ValueOf(stream))
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "close send failed" {
			t.Errorf("error mismatch: got %q, want %q", err.Error(), "close send failed")
		}
	})

	t.Run("no CloseSend method", func(t *testing.T) {
		stream := struct{}{}
		err := GRPCStreamCloseSend(reflect.ValueOf(&stream))
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "stream does not have CloseSend method" {
			t.Errorf("error mismatch: got %q", err.Error())
		}
	})
}

func TestGRPCStreamRequestType(t *testing.T) {
	client := testpb.NewTestClient(nil)
	v := reflect.ValueOf(client)

	t.Run("server stream", func(t *testing.T) {
		method := v.MethodByName("ServerStreamEcho")
		typ, err := GRPCStreamRequestType(method, GRPCMethodServerStream)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := reflect.TypeFor[testpb.EchoRequest]()
		if typ != expected {
			t.Errorf("type mismatch: got %v, want %v", typ, expected)
		}
	})

	t.Run("client stream", func(t *testing.T) {
		method := v.MethodByName("ClientStreamEcho")
		typ, err := GRPCStreamRequestType(method, GRPCMethodClientStream)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := reflect.TypeFor[testpb.EchoRequest]()
		if typ != expected {
			t.Errorf("type mismatch: got %v, want %v", typ, expected)
		}
	})

	t.Run("bidi stream", func(t *testing.T) {
		method := v.MethodByName("BidiStreamEcho")
		typ, err := GRPCStreamRequestType(method, GRPCMethodBidiStream)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := reflect.TypeFor[testpb.EchoRequest]()
		if typ != expected {
			t.Errorf("type mismatch: got %v, want %v", typ, expected)
		}
	})

	t.Run("default falls back to In(1)", func(t *testing.T) {
		// Unary method type uses the default case
		method := v.MethodByName("Echo")
		typ, err := GRPCStreamRequestType(method, GRPCMethodUnary)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := reflect.TypeFor[testpb.EchoRequest]()
		if typ != expected {
			t.Errorf("type mismatch: got %v, want %v", typ, expected)
		}
	})

	t.Run("client stream without Send method", func(t *testing.T) {
		// A method returning a type without Send
		type noSendStream struct{ grpc.ClientStream }
		mockMethod := reflect.ValueOf(func(ctx gocontext.Context, opts ...grpc.CallOption) (*noSendStream, error) {
			return nil, nil //nolint:nilnil
		})
		_, err := GRPCStreamRequestType(mockMethod, GRPCMethodClientStream)
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "stream type does not have Send method" {
			t.Errorf("error mismatch: got %q", err.Error())
		}
	})
}

func TestGRPCStreamAsClientStream(t *testing.T) {
	t.Run("valid client stream", func(t *testing.T) {
		stream := &mockClientStream{}
		cs, ok := GRPCStreamAsClientStream(reflect.ValueOf(stream))
		if !ok {
			t.Fatal("expected ok to be true")
		}
		if cs == nil {
			t.Fatal("expected non-nil ClientStream")
		}
	})

	t.Run("not a client stream", func(t *testing.T) {
		stream := &mockStream{}
		_, ok := GRPCStreamAsClientStream(reflect.ValueOf(stream))
		if ok {
			t.Fatal("expected ok to be false for non-ClientStream")
		}
	})

	t.Run("nil value", func(t *testing.T) {
		_, ok := GRPCStreamAsClientStream(reflect.ValueOf((*mockClientStream)(nil)))
		if ok {
			t.Fatal("expected ok to be false for nil")
		}
	})

	t.Run("invalid value", func(t *testing.T) {
		_, ok := GRPCStreamAsClientStream(reflect.Value{})
		if ok {
			t.Fatal("expected ok to be false for invalid reflect.Value")
		}
	})
}
