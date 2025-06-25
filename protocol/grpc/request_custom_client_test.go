package grpc

import (
	"strings"
	"testing"

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
