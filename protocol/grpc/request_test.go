package grpc

import (
	"bytes"
	gocontext "context"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/internal/testutil"
	"github.com/zoncoen/scenarigo/internal/testutil/gen/pb/test"
	"github.com/zoncoen/scenarigo/reporter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRequest_Invoke(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Run("Echo returns no error", func(t *testing.T) {
			req := &test.EchoRequest{MessageId: "1", MessageBody: "hello"}
			resp := &test.EchoResponse{MessageId: "1", MessageBody: "hello"}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := test.NewMockTestClient(ctrl)
			client.EXPECT().Echo(gomock.Any(), req, gomock.Any()).Return(resp, nil)

			r := &Request{
				Client: "{{vars.client}}",
				Method: "Echo",
				Body: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
			}
			ctx := context.FromT(t).WithVars(map[string]interface{}{
				"client": client,
			})
			ctx, result, err := r.Invoke(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			typedResult, ok := result.(response)
			if !ok {
				t.Fatalf("failed to type conversion from %s to response", reflect.TypeOf(result))
			}
			message, serr, err := extract(typedResult)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(resp, message); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
			if serr != nil {
				t.Fatalf("unexpected error: %v", serr)
			}

			// ensure that ctx.WithRequest and ctx.WithResponse are called
			if diff := cmp.Diff(req, ctx.Request()); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
			respValue, ok := ctx.Response().(response)
			if !ok {
				t.Fatal("failed to get response type from ctx.Response()")
			}
			if diff := cmp.Diff(resp, respValue.Body); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
		})
		t.Run("Echo returns error", func(t *testing.T) {
			req := &test.EchoRequest{MessageId: "1", MessageBody: "hello"}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := test.NewMockTestClient(ctrl)
			client.EXPECT().Echo(gomock.Any(), req, gomock.Any()).Return(nil, status.New(codes.Unauthenticated, "unauthenticated").Err())

			r := &Request{
				Client: "{{vars.client}}",
				Method: "Echo",
				Body: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
			}
			ctx := context.FromT(t).WithVars(map[string]interface{}{
				"client": client,
			})
			ctx, result, err := r.Invoke(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			typedResult, ok := result.(response)
			if !ok {
				t.Fatalf("failed to type conversion from %s to response", reflect.TypeOf(result))
			}
			_, serr, err := extract(typedResult)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if serr.Code() != codes.Unauthenticated {
				t.Fatalf("expected code is %s but got %s", codes.Unauthenticated.String(), serr.Code().String())
			}

			// ensure that ctx.WithRequest and ctx.WithResponse are called
			if diff := cmp.Diff(req, ctx.Request()); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
		})
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			vars        map[string]interface{}
			client      string
			method      string
			expectError string
		}{
			"no client": {
				expectError: "gRPC client must be specified",
			},
			"client not found": {
				client:      "{{vars.client}}",
				expectError: "failed to get client",
			},
			"nil client": {
				vars: map[string]interface{}{
					"client": nil,
				},
				client:      "{{vars.client}}",
				method:      "Echo",
				expectError: "client {{vars.client}} is invalid",
			},
			"method not found": {
				vars: map[string]interface{}{
					"client": test.NewTestClient(nil),
				},
				client:      "{{vars.client}}",
				method:      "NotFound",
				expectError: "method {{vars.client}}.NotFound not found",
			},
		}
		for name, tc := range tests {
			tc := tc
			t.Run(name, func(t *testing.T) {
				ctx := context.FromT(t)
				if tc.vars != nil {
					ctx = ctx.WithVars(tc.vars)
				}
				req := &Request{
					Client: tc.client,
					Method: tc.method,
				}
				_, _, err := req.Invoke(ctx)
				if err == nil {
					t.Fatal("no error")
				}
				if e := err.Error(); !strings.Contains(e, tc.expectError) {
					t.Errorf(`"%s" does not contain "%s"`, e, tc.expectError)
				}
			})
		}
	})
}

func TestRequest_Invoke_Log(t *testing.T) {
	req := &test.EchoRequest{MessageId: "1", MessageBody: "hello"}
	resp := &test.EchoResponse{MessageId: "1", MessageBody: "hello"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := test.NewMockTestClient(ctrl)
	client.EXPECT().Echo(gomock.Any(), req, gomock.Any()).Return(resp, nil)

	r := &Request{
		Client: "{{vars.client}}",
		Method: "Echo",
		Metadata: map[string]string{
			"version": "1.0.0",
		},
		Body: yaml.MapSlice{
			yaml.MapItem{Key: "messageId", Value: "1"},
			yaml.MapItem{Key: "messageBody", Value: "hello"},
		},
	}

	var b bytes.Buffer
	reporter.Run(func(rptr reporter.Reporter) {
		rptr.Run("test.yaml", func(rptr reporter.Reporter) {
			ctx := context.New(rptr).WithVars(map[string]interface{}{
				"client": client,
			})
			if _, _, err := r.Invoke(ctx); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}, reporter.WithWriter(&b), reporter.WithVerboseLog())

	expect := `
=== RUN   test.yaml
--- PASS: test.yaml (0.00s)
    request:
        method: Echo
        metadata:
          version:
          - 1.0.0
        body:
          messageId: "1"
          messageBody: hello
    response:
        body:
          messageId: "1"
          messageBody: hello
PASS
ok  	test.yaml	0.000s
`
	if diff := cmp.Diff(expect, "\n"+testutil.ResetDuration(b.String())); diff != "" {
		t.Errorf("differs (-want +got):\n%s", diff)
	}
}

func TestValidateMethod(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		method := reflect.ValueOf(test.NewTestClient(nil)).MethodByName("Echo")
		if err := validateMethod(method); err != nil {
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
					return nil, nil
				}),
			},
			"first argument must be context.Context": {
				method: reflect.ValueOf(func(ctx struct{}, in proto.Message, opts ...grpc.CallOption) (proto.Message, error) {
					return nil, nil
				}),
			},
			"second argument must be proto.Message": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in struct{}, opts ...grpc.CallOption) (proto.Message, error) {
					return nil, nil
				}),
			},
			"third argument must be []grpc.CallOption": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in proto.Message, opts ...struct{}) (proto.Message, error) {
					return nil, nil
				}),
			},
			"number of return values must be 2": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in proto.Message, opts ...grpc.CallOption) {
				}),
			},
			"first return value must be proto.Message": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in proto.Message, opts ...grpc.CallOption) (*struct{}, error) {
					return nil, nil
				}),
			},
			"second return value must be error": {
				method: reflect.ValueOf(func(ctx gocontext.Context, in proto.Message, opts ...grpc.CallOption) (proto.Message, *struct{}) {
					return nil, nil
				}),
			},
		}
		for name, tc := range tests {
			tc := tc
			t.Run(name, func(t *testing.T) {
				if err := validateMethod(tc.method); err == nil {
					t.Fatal("no error")
				}
			})
		}
	})
}

func TestBuildRequestBody(t *testing.T) {
	tests := map[string]struct {
		vars   interface{}
		src    interface{}
		expect *test.EchoRequest
		error  bool
	}{
		"empty": {
			expect: &test.EchoRequest{},
		},
		"set fields": {
			src: yaml.MapSlice{
				yaml.MapItem{
					Key:   "messageId",
					Value: "1",
				},
				yaml.MapItem{
					Key:   "messageBody",
					Value: "hello",
				},
			},
			expect: &test.EchoRequest{
				MessageId:   "1",
				MessageBody: "hello",
			},
		},
		"with vars": {
			vars: map[string]string{
				"body": "hello",
			},
			src: yaml.MapSlice{
				yaml.MapItem{
					Key:   "messageBody",
					Value: "{{vars.body}}",
				},
			},
			expect: &test.EchoRequest{
				MessageBody: "hello",
			},
		},
		"unknown field": {
			src: yaml.MapSlice{
				yaml.MapItem{
					Key: "invalid",
				},
			},
			error: true,
		},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			ctx := context.FromT(t)
			if tc.vars != nil {
				ctx = ctx.WithVars(tc.vars)
			}
			var req test.EchoRequest
			err := buildRequestBody(ctx, &req, tc.src)
			if err != nil {
				if !tc.error {
					t.Fatalf("unexpected error: %s", err)
				}
				return
			}
			if tc.error {
				t.Fatal("no error")
			}
			if diff := cmp.Diff(tc.expect, &req); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestNormalizeRequestBodyValue(t *testing.T) {
	invalidRequestBody := func() {}
	if _, err := normalizeValue(invalidRequestBody); err == nil {
		t.Fatal("no error")
	}
}
