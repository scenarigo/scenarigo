package grpc

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/mockutil"
	"github.com/scenarigo/scenarigo/internal/plugin"
	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

func TestNewCustomServiceClient(t *testing.T) {
	tests := map[string]struct {
		r           *Request
		v           any
		expectError string
	}{
		"success": {
			r: &Request{
				Client: "{{vars.client}}",
				Method: "Echo",
			},
			v: testpb.NewTestClient(nil),
		},
		"success server stream": {
			r: &Request{
				Client: "{{vars.client}}",
				Method: "ServerStreamEcho",
			},
			v: testpb.NewTestClient(nil),
		},
		"success client stream": {
			r: &Request{
				Client: "{{vars.client}}",
				Method: "ClientStreamEcho",
			},
			v: testpb.NewTestClient(nil),
		},
		"success bidi stream": {
			r: &Request{
				Client: "{{vars.client}}",
				Method: "BidiStreamEcho",
			},
			v: testpb.NewTestClient(nil),
		},
		"invalid client": {
			r: &Request{
				Client: "{{vars.client}}",
				Method: "Echo",
			},
			expectError: `.client: client "{{vars.client}}" is invalid`,
		},
		"method not found": {
			r: &Request{
				Client: "{{vars.client}}",
				Method: "Invalid",
			},
			v:           testpb.NewTestClient(nil),
			expectError: `.method: method "{{vars.client}}.Invalid" not found`,
		},
		"invalid method": {
			r: &Request{
				Client: "{{vars.client}}",
				Method: "String",
			},
			v:           &testpb.EchoRequest{},
			expectError: `.method: "{{vars.client}}.String" must be "func(context.Context, proto.Message, ...grpc.CallOption) (proto.Message, error): number of arguments must be 3 but got 0`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newCustomServiceClient(test.r, test.v)
			if err != nil {
				if test.expectError == "" {
					t.Fatalf("unexpected error: %s", err)
				}
			} else {
				if test.expectError != "" && !strings.Contains(err.Error(), test.expectError) {
					t.Fatalf("expected error %q but got %q", test.expectError, err.Error())
				}
			}
		})
	}
}

func TestNewCustomServiceClient_MethodType(t *testing.T) {
	client := testpb.NewTestClient(nil)
	tests := map[string]struct {
		method     string
		expectType plugin.GRPCMethodType
	}{
		"unary": {
			method:     "Echo",
			expectType: plugin.GRPCMethodUnary,
		},
		"server stream": {
			method:     "ServerStreamEcho",
			expectType: plugin.GRPCMethodServerStream,
		},
		"client stream": {
			method:     "ClientStreamEcho",
			expectType: plugin.GRPCMethodClientStream,
		},
		"bidi stream": {
			method:     "BidiStreamEcho",
			expectType: plugin.GRPCMethodBidiStream,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sc, err := newCustomServiceClient(&Request{
				Client: "{{vars.client}}",
				Method: test.method,
			}, client)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if sc.methodType != test.expectType {
				t.Errorf("expected method type %d but got %d", test.expectType, sc.methodType)
			}
		})
	}
}

func TestCustomServiceClient_ServerStream(t *testing.T) {
	resp1 := &testpb.EchoResponse{MessageId: "1", MessageBody: "hello"}
	resp2 := &testpb.EchoResponse{MessageId: "2", MessageBody: "world"}
	req := &testpb.EchoRequest{MessageId: "1", MessageBody: "hello"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	streamClient := testpb.NewMockTest_ServerStreamEchoClient(ctrl)
	streamClient.EXPECT().Recv().Return(resp1, nil)
	streamClient.EXPECT().Recv().Return(resp2, nil)
	streamClient.EXPECT().Recv().Return(nil, io.EOF)
	streamClient.EXPECT().Header().Return(metadata.MD{"key": {"val"}}, nil)
	streamClient.EXPECT().Trailer().Return(metadata.MD{"tkey": {"tval"}})

	client := testpb.NewMockTestClient(ctrl)
	client.EXPECT().ServerStreamEcho(gomock.Any(), mockutil.ProtoMessage(req), gomock.Any()).Return(streamClient, nil)

	r := &Request{
		Client: "{{vars.client}}",
		Method: "ServerStreamEcho",
		Message: yaml.MapSlice{
			yaml.MapItem{Key: "messageId", Value: "1"},
			yaml.MapItem{Key: "messageBody", Value: "hello"},
		},
	}
	ctx := context.FromT(t).WithVars(map[string]any{
		"client": client,
	})
	_, result, err := r.Invoke(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	typedResult, ok := result.(*response)
	if !ok {
		t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
	}
	if len(typedResult.Messages) != 2 {
		t.Fatalf("expected 2 response messages but got %d", len(typedResult.Messages))
	}
	if diff := cmp.Diff(resp1, typedResult.Messages[0].Message, protocmp.Transform()); diff != "" {
		t.Errorf("response[0] differs: (-want +got)\n%s", diff)
	}
	if diff := cmp.Diff(resp2, typedResult.Messages[1].Message, protocmp.Transform()); diff != "" {
		t.Errorf("response[1] differs: (-want +got)\n%s", diff)
	}
}

func TestCustomServiceClient_ServerStream_Error(t *testing.T) {
	req := &testpb.EchoRequest{MessageId: "1", MessageBody: "hello"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	streamClient := testpb.NewMockTest_ServerStreamEchoClient(ctrl)
	streamClient.EXPECT().Recv().Return(nil, status.Error(codes.Internal, "internal error"))
	streamClient.EXPECT().Header().Return(nil, nil)
	streamClient.EXPECT().Trailer().Return(nil)

	client := testpb.NewMockTestClient(ctrl)
	client.EXPECT().ServerStreamEcho(gomock.Any(), mockutil.ProtoMessage(req), gomock.Any()).Return(streamClient, nil)

	r := &Request{
		Client: "{{vars.client}}",
		Method: "ServerStreamEcho",
		Message: yaml.MapSlice{
			yaml.MapItem{Key: "messageId", Value: "1"},
			yaml.MapItem{Key: "messageBody", Value: "hello"},
		},
	}
	ctx := context.FromT(t).WithVars(map[string]any{
		"client": client,
	})
	_, result, err := r.Invoke(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	typedResult, ok := result.(*response)
	if !ok {
		t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
	}
	if typedResult.Status.Code() != codes.Internal {
		t.Fatalf("expected code Internal but got %s", typedResult.Status.Code().String())
	}
}

func TestCustomServiceClient_ClientStream(t *testing.T) {
	resp := &testpb.EchoResponse{MessageId: "1", MessageBody: "hello world"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	streamClient := testpb.NewMockTest_ClientStreamEchoClient(ctrl)
	streamClient.EXPECT().Send(mockutil.ProtoMessage(&testpb.EchoRequest{MessageId: "1", MessageBody: "hello"})).Return(nil)
	streamClient.EXPECT().Send(mockutil.ProtoMessage(&testpb.EchoRequest{MessageId: "2", MessageBody: "world"})).Return(nil)
	streamClient.EXPECT().CloseAndRecv().Return(resp, nil)
	streamClient.EXPECT().Header().Return(metadata.MD{"key": {"val"}}, nil)
	streamClient.EXPECT().Trailer().Return(metadata.MD{"tkey": {"tval"}})

	client := testpb.NewMockTestClient(ctrl)
	client.EXPECT().ClientStreamEcho(gomock.Any(), gomock.Any()).Return(streamClient, nil)

	r := &Request{
		Client: "{{vars.client}}",
		Method: "ClientStreamEcho",
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
	}
	ctx := context.FromT(t).WithVars(map[string]any{
		"client": client,
	})
	_, result, err := r.Invoke(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	typedResult, ok := result.(*response)
	if !ok {
		t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
	}
	if diff := cmp.Diff(resp, typedResult.Message.Message, protocmp.Transform()); diff != "" {
		t.Errorf("response differs: (-want +got)\n%s", diff)
	}
	if typedResult.Status.Code() != codes.OK {
		t.Fatalf("expected OK but got %s", typedResult.Status.Code().String())
	}
}

func TestCustomServiceClient_ClientStream_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	streamClient := testpb.NewMockTest_ClientStreamEchoClient(ctrl)
	streamClient.EXPECT().Send(gomock.Any()).Return(nil)
	streamClient.EXPECT().Send(gomock.Any()).Return(nil)
	streamClient.EXPECT().CloseAndRecv().Return(nil, status.Error(codes.InvalidArgument, "invalid"))
	streamClient.EXPECT().Header().Return(nil, nil)
	streamClient.EXPECT().Trailer().Return(nil)

	client := testpb.NewMockTestClient(ctrl)
	client.EXPECT().ClientStreamEcho(gomock.Any(), gomock.Any()).Return(streamClient, nil)

	r := &Request{
		Client: "{{vars.client}}",
		Method: "ClientStreamEcho",
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
	}
	ctx := context.FromT(t).WithVars(map[string]any{
		"client": client,
	})
	_, result, err := r.Invoke(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	typedResult, ok := result.(*response)
	if !ok {
		t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
	}
	if typedResult.Status.Code() != codes.InvalidArgument {
		t.Fatalf("expected code InvalidArgument but got %s", typedResult.Status.Code().String())
	}
}

func TestCustomServiceClient_BidiStream(t *testing.T) {
	resp1 := &testpb.EchoResponse{MessageId: "1", MessageBody: "hello"}
	resp2 := &testpb.EchoResponse{MessageId: "2", MessageBody: "world"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	streamClient := testpb.NewMockTest_BidiStreamEchoClient(ctrl)
	streamClient.EXPECT().Send(mockutil.ProtoMessage(&testpb.EchoRequest{MessageId: "1", MessageBody: "hello"})).Return(nil)
	streamClient.EXPECT().Send(mockutil.ProtoMessage(&testpb.EchoRequest{MessageId: "2", MessageBody: "world"})).Return(nil)
	streamClient.EXPECT().CloseSend().Return(nil)
	streamClient.EXPECT().Recv().Return(resp1, nil)
	streamClient.EXPECT().Recv().Return(resp2, nil)
	streamClient.EXPECT().Recv().Return(nil, io.EOF)
	streamClient.EXPECT().Header().Return(metadata.MD{"key": {"val"}}, nil)
	streamClient.EXPECT().Trailer().Return(metadata.MD{"tkey": {"tval"}})

	client := testpb.NewMockTestClient(ctrl)
	client.EXPECT().BidiStreamEcho(gomock.Any(), gomock.Any()).Return(streamClient, nil)

	r := &Request{
		Client: "{{vars.client}}",
		Method: "BidiStreamEcho",
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
	}
	ctx := context.FromT(t).WithVars(map[string]any{
		"client": client,
	})
	_, result, err := r.Invoke(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	typedResult, ok := result.(*response)
	if !ok {
		t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
	}
	if len(typedResult.Messages) != 2 {
		t.Fatalf("expected 2 response messages but got %d", len(typedResult.Messages))
	}
	if diff := cmp.Diff(resp1, typedResult.Messages[0].Message, protocmp.Transform()); diff != "" {
		t.Errorf("response[0] differs: (-want +got)\n%s", diff)
	}
	if diff := cmp.Diff(resp2, typedResult.Messages[1].Message, protocmp.Transform()); diff != "" {
		t.Errorf("response[1] differs: (-want +got)\n%s", diff)
	}
}

func TestCustomServiceClient_BidiStream_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	streamClient := testpb.NewMockTest_BidiStreamEchoClient(ctrl)
	streamClient.EXPECT().Send(gomock.Any()).Return(nil)
	streamClient.EXPECT().CloseSend().Return(nil)
	streamClient.EXPECT().Recv().Return(nil, status.Error(codes.Internal, "internal error"))
	streamClient.EXPECT().Header().Return(nil, nil)
	streamClient.EXPECT().Trailer().Return(nil)

	client := testpb.NewMockTestClient(ctrl)
	client.EXPECT().BidiStreamEcho(gomock.Any(), gomock.Any()).Return(streamClient, nil)

	r := &Request{
		Client: "{{vars.client}}",
		Method: "BidiStreamEcho",
		Messages: []any{
			yaml.MapSlice{
				yaml.MapItem{Key: "messageId", Value: "1"},
				yaml.MapItem{Key: "messageBody", Value: "hello"},
			},
		},
	}
	ctx := context.FromT(t).WithVars(map[string]any{
		"client": client,
	})
	_, result, err := r.Invoke(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	typedResult, ok := result.(*response)
	if !ok {
		t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
	}
	if typedResult.Status.Code() != codes.Internal {
		t.Fatalf("expected code Internal but got %s", typedResult.Status.Code().String())
	}
}
