package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/scenarigo/scenarigo/internal/yamlutil"
	"github.com/scenarigo/scenarigo/mock/protocol"
	"github.com/scenarigo/scenarigo/protocol/grpc/proto"
)

func TestUnaryHandler_failure(t *testing.T) {
	comp := proto.NewCompiler(nil)
	fds, err := comp.Compile(context.Background(), []string{"./testdata/test.proto"})
	if err != nil {
		t.Fatalf("failed to compile proto: %s", err)
	}
	svcName := protoreflect.FullName("scenarigo.testdata.test.Test")
	sd, err := fds.ResolveService(svcName)
	if err != nil {
		t.Fatalf("failed to resolve service: %s", err)
	}
	md := sd.Methods().ByName("Echo")

	tests := map[string]struct {
		mocks   []protocol.Mock
		svcName protoreflect.FullName
		method  protoreflect.MethodDescriptor
		decode  func(any) error
		expect  string
	}{
		"no mock": {
			expect: "failed to get mock: no mocks remain",
		},
		"protocol must be grpc": {
			mocks: []protocol.Mock{
				{
					Protocol: "http",
				},
			},
			expect: `received gRPC request but the mock protocol is "http"`,
		},
		"failed to unmarshal expect": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage("-"),
				},
			},
			expect: "failed to unmarshal: [1:1] sequence was used where mapping is expected",
		},
		"invalid expect service": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage(`service: '{{'`),
				},
			},
			expect: ".expect.service: failed to build assretion: invalid expect service: failed to build assertion",
		},
		"invalid expect method": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage(`method: '{{'`),
				},
			},
			expect: ".expect.method: failed to build assretion: invalid expect method: failed to build assertion",
		},
		"invalid expect metadata": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage("metadata:\n  foo: '{{'"),
				},
			},
			expect: ".expect.metadata.foo: failed to build assretion: invalid expect metadata: failed to build assertion",
		},
		"invalid expect message": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage(`message: '{{'`),
				},
			},
			expect: ".expect.message: failed to build assretion: invalid expect response message: failed to build assertion",
		},
		"failed to decode message": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage(""),
				},
			},
			svcName: svcName,
			method:  md,
			decode:  func(_ any) error { return errors.New("ERROR") },
			expect:  ".expect.message: failed to decode message: ERROR",
		},
		"assertion error": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage("message:\n  messageId: '1'"),
				},
			},
			svcName: svcName,
			method:  md,
			decode:  func(_ any) error { return nil },
			expect:  `request assertion failed: expected "1" but got ""`,
		},
		"failed to unmarshal response": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage(""),
					Response: yamlutil.RawMessage("-"),
				},
			},
			svcName: svcName,
			method:  md,
			decode:  func(_ any) error { return nil },
			expect:  ".response: failed to unmarshal response: [1:1] sequence was used where mapping is expected",
		},
		"failed to execute template of response": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage(""),
					Response: yamlutil.RawMessage("message: '{{'"),
				},
			},
			svcName: svcName,
			method:  md,
			decode:  func(_ any) error { return nil },
			expect:  ".response.message: failed to execute template of response",
		},
		"invalid response status code": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage(""),
					Response: yamlutil.RawMessage("status:\n  code: aaa"),
				},
			},
			svcName: svcName,
			method:  md,
			decode:  func(_ any) error { return nil },
			expect:  ".response.status.code: invalid status code",
		},
		"invalid response message": {
			mocks: []protocol.Mock{
				{
					Protocol: "grpc",
					Expect:   yamlutil.RawMessage(""),
					Response: yamlutil.RawMessage("message:\n  id: '1'"),
				},
			},
			svcName: svcName,
			method:  md,
			decode:  func(_ any) error { return nil },
			expect:  ".response.message: invalid message",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			iter := protocol.NewMockIterator(test.mocks)
			srv := &server{
				iter: iter,
			}
			ctx := context.Background()
			if _, err := srv.unaryHandler(test.svcName, test.method)(nil, ctx, test.decode, nil); err == nil {
				t.Fatal("no error")
			} else if !strings.Contains(err.Error(), test.expect) {
				t.Errorf("expect error %q but got %q", test.expect, err)
			}
		})
	}
}
