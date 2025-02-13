package grpc

import (
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

func TestEqualEnum(t *testing.T) {
	tests := map[string]struct {
		expected interface{}
		got      interface{}
		ok       bool
	}{
		"expected is not string": {
			expected: 1,
			got:      test.UserType_CUSTOMER,
		},
		"got is not enum": {
			expected: "CUSTOMER",
			got:      1,
		},
		"equals": {
			expected: "CUSTOMER",
			got:      test.UserType_CUSTOMER,
			ok:       true,
		},
		"not equals": {
			expected: "CUSTOMER",
			got:      test.UserType_STAFF,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			ok, err := equalEnum(test.expected, test.got)
			if ok != test.ok {
				t.Errorf("expect %t but got %t", test.ok, ok)
			}
			if err != nil {
				t.Error(err)
			}
		})
	}
}

func TestEqualMessage(t *testing.T) {
	tests := map[string]struct {
		expected interface{}
		got      interface{}
		ok       bool
	}{
		"expected is not proto message": {
			expected: "",
			got:      &test.EchoResponse{},
		},
		"got is not proto message": {
			expected: &test.EchoResponse{},
			got:      "",
		},
		"equals": {
			expected: echoResponse(t, "xxx", "hello"),
			got: &test.EchoResponse{
				MessageId:   "xxx",
				MessageBody: "hello",
			},
			ok: true,
		},
		"equals (convert)": {
			expected: echoResponse(t, "xxx", "hello"),
			got: test.EchoResponse{
				MessageId:   "xxx",
				MessageBody: "hello",
			},
			ok: true,
		},
		"not equals": {
			expected: &test.EchoResponse{
				MessageId:   "xxx",
				MessageBody: "hello",
			},
			got: &test.EchoResponse{
				MessageId:   "yyy",
				MessageBody: "bye",
			},
		},
		"untyped nil doesn't implement proto.Message": {
			expected: nil,
			got:      nil,
		},
		"typed nil implements proto.Message": {
			expected: (*test.EchoResponse)(nil),
			got:      (*test.EchoResponse)(nil),
			ok:       true,
		},
		"different type": {
			expected: (*test.EchoRequest)(nil),
			got:      (*test.EchoResponse)(nil),
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			ok, err := equalMessage(test.expected, test.got)
			if ok != test.ok {
				t.Errorf("expect %t but got %t", test.ok, ok)
			}
			if err != nil {
				t.Error(err)
			}
		})
	}
}

func echoResponse(t *testing.T, id, body string) *test.EchoResponse {
	t.Helper()

	var msg test.EchoResponse
	if err := protojson.Unmarshal([]byte(fmt.Sprintf(`
{
  "messageId": "%s",
  "messageBody": "%s"
}
`, id, body)), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %s", err)
	}

	// Ensure at least one of the unexported fields is not zero value.
	//nolint:govet
	if reflect.DeepEqual(msg, test.EchoResponse{
		MessageId:   id,
		MessageBody: body,
	}) {
		t.Fatal("expect not deep equal for testing")
	}

	return &msg
}
