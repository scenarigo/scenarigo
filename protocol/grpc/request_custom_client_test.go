package grpc

import (
	gocontext "context"
	"reflect"
	"strings"
	"testing"

	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func TestNewCustomServiceClient(t *testing.T) {
	tests := map[string]struct {
		r           *Request
		v           reflect.Value
		expectError string
	}{
		"success": {
			r: &Request{
				Client: "{{vars.client}}",
				Method: "Echo",
			},
			v: reflect.ValueOf(testpb.NewTestClient(nil)),
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
			v:           reflect.ValueOf(testpb.NewTestClient(nil)),
			expectError: `.method: method "{{vars.client}}.Invalid" not found`,
		},
		"invalid method": {
			r: &Request{
				Client: "{{vars.client}}",
				Method: "String",
			},
			v:           reflect.ValueOf(&testpb.EchoRequest{}),
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

func TestValidateMethod(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		method := reflect.ValueOf(testpb.NewTestClient(nil)).MethodByName("Echo")
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
			tc := tc
			t.Run(name, func(t *testing.T) {
				if err := validateMethod(tc.method); err == nil {
					t.Fatal("no error")
				}
			})
		}
	})
}
