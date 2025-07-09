package plugin

import (
	gocontext "context"
	"errors"
	"reflect"
	"testing"

	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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
