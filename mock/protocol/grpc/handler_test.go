package grpc

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/scenarigo/scenarigo/internal/yamlutil"
	"github.com/scenarigo/scenarigo/mock/protocol"
	grpcprotocol "github.com/scenarigo/scenarigo/protocol/grpc"
	"github.com/scenarigo/scenarigo/protocol/grpc/proto"
)

// mockStream implements grpc.ServerStream for testing.
type mockStream struct {
	ctx      context.Context
	recvMsgs []any // messages to return from RecvMsg (nil entry means use the passed msg)
	recvErr  error // error to return from RecvMsg when recvMsgs is exhausted
	sentMsgs []any // messages sent via SendMsg
	sendErr  error // error to return from SendMsg
	recvIdx  int
}

func (s *mockStream) SetHeader(metadata.MD) error  { return nil }
func (s *mockStream) SendHeader(metadata.MD) error { return nil }
func (s *mockStream) SetTrailer(metadata.MD)       {}
func (s *mockStream) Context() context.Context     { return s.ctx }
func (s *mockStream) SendMsg(m any) error          { s.sentMsgs = append(s.sentMsgs, m); return s.sendErr }
func (s *mockStream) RecvMsg(m any) error {
	if s.recvIdx < len(s.recvMsgs) {
		// Copy fields from recvMsgs to m if both are dynamic messages
		if src, ok := s.recvMsgs[s.recvIdx].(*dynamicpb.Message); ok {
			if dst, ok := m.(*dynamicpb.Message); ok {
				src.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
					dst.Set(fd, v)
					return true
				})
			}
		}
		s.recvIdx++
		return nil
	}
	if s.recvErr != nil {
		return s.recvErr
	}
	return io.EOF
}

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

func TestStreamHandler_failure(t *testing.T) {
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
	serverStreamMD := sd.Methods().ByName("ServerStreamEcho")
	clientStreamMD := sd.Methods().ByName("ClientStreamEcho")
	bidiStreamMD := sd.Methods().ByName("BidiStreamEcho")

	// Helper to create a dynamic message with fields set
	newMsg := func(md protoreflect.MethodDescriptor) *dynamicpb.Message {
		return dynamicpb.NewMessage(md.Input())
	}

	tests := map[string]struct {
		mocks  []protocol.Mock
		method protoreflect.MethodDescriptor
		stream *mockStream
		expect string
	}{
		// streamHandler common errors
		"no mock": {
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "failed to get mock: no mocks remain",
		},
		"protocol must be grpc": {
			mocks:  []protocol.Mock{{Protocol: "http"}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: `received gRPC request but the mock protocol is "http"`,
		},
		"failed to unmarshal expect": {
			mocks:  []protocol.Mock{{Protocol: "grpc", Expect: yamlutil.RawMessage("-")}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "failed to unmarshal",
		},
		"failed to unmarshal response": {
			mocks:  []protocol.Mock{{Protocol: "grpc", Expect: yamlutil.RawMessage(""), Response: yamlutil.RawMessage("-")}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "failed to unmarshal response",
		},

		// server streaming errors
		"server stream: invalid expect": {
			mocks:  []protocol.Mock{{Protocol: "grpc", Expect: yamlutil.RawMessage(`service: '{{'`), Response: yamlutil.RawMessage("")}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background(), recvMsgs: []any{newMsg(serverStreamMD)}},
			expect: ".expect.service: failed to build assertion: invalid expect service",
		},
		"server stream: assertion error": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage("message:\n  messageId: '1'"),
				Response: yamlutil.RawMessage(""),
			}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background(), recvMsgs: []any{newMsg(serverStreamMD)}},
			expect: "request assertion failed",
		},
		"server stream: invalid status code": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("status:\n  code: aaa"),
			}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background(), recvMsgs: []any{newMsg(serverStreamMD)}},
			expect: "invalid status code",
		},
		"server stream: invalid response message": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("messages:\n  - id: '1'"),
			}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background(), recvMsgs: []any{newMsg(serverStreamMD)}},
			expect: "invalid message",
		},
		"server stream: failed to execute template": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("messages:\n  - messageId: '{{'"),
			}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background(), recvMsgs: []any{newMsg(serverStreamMD)}},
			expect: "failed to execute template",
		},
		"server stream: recv error": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage(""),
			}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background(), recvErr: errors.New("recv failed")},
			expect: "failed to receive message",
		},
		"server stream: non-OK status": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("status:\n  code: NotFound"),
			}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background(), recvMsgs: []any{newMsg(serverStreamMD)}},
			expect: "NotFound",
		},
		"server stream: non-OK status with message": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("status:\n  code: NotFound\n  message: custom error"),
			}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background(), recvMsgs: []any{newMsg(serverStreamMD)}},
			expect: "custom error",
		},
		"server stream: send error": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("messages:\n  - messageId: '1'"),
			}},
			method: serverStreamMD,
			stream: &mockStream{ctx: context.Background(), recvMsgs: []any{newMsg(serverStreamMD)}, sendErr: errors.New("send failed")},
			expect: "failed to send message",
		},

		// client streaming errors
		"client stream: recv error": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage(""),
			}},
			method: clientStreamMD,
			stream: &mockStream{ctx: context.Background(), recvErr: errors.New("recv failed")},
			expect: "failed to receive message",
		},
		"client stream: invalid expect": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(`service: '{{'`),
				Response: yamlutil.RawMessage(""),
			}},
			method: clientStreamMD,
			stream: &mockStream{ctx: context.Background()}, // no messages, immediate EOF
			expect: "failed to build assertion: invalid expect service",
		},
		"client stream: assertion error": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage("messages:\n  - messageId: '1'"),
				Response: yamlutil.RawMessage(""),
			}},
			method: clientStreamMD,
			stream: &mockStream{ctx: context.Background()}, // no messages
			expect: "request assertion failed",
		},
		"client stream: invalid status code": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("status:\n  code: aaa"),
			}},
			method: clientStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "invalid status code",
		},
		"client stream: failed to execute template": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("message: '{{'"),
			}},
			method: clientStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "failed to execute template of response",
		},
		"client stream: invalid response message": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("message:\n  id: '1'"),
			}},
			method: clientStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "invalid message",
		},
		"client stream: non-OK status": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("status:\n  code: PermissionDenied"),
			}},
			method: clientStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "PermissionDenied",
		},
		"client stream: non-OK status with message": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("status:\n  code: PermissionDenied\n  message: denied"),
			}},
			method: clientStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "denied",
		},

		// bidi streaming errors
		"bidi stream: invalid status code": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("status:\n  code: aaa"),
			}},
			method: bidiStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "invalid status code",
		},
		"bidi stream: failed to execute template": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("messages:\n  - messageId: '{{'"),
			}},
			method: bidiStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "failed to execute template",
		},
		"bidi stream: invalid response message": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("messages:\n  - id: '1'"),
			}},
			method: bidiStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "invalid message",
		},
		"bidi stream: assertion error": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage("messages:\n  - messageId: '1'"),
				Response: yamlutil.RawMessage(""),
			}},
			method: bidiStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "request assertion failed",
		},
		"bidi stream: invalid expect": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(`service: '{{'`),
				Response: yamlutil.RawMessage(""),
			}},
			method: bidiStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "failed to build assertion: invalid expect service",
		},
		"bidi stream: non-OK status": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("status:\n  code: Unavailable"),
			}},
			method: bidiStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "Unavailable",
		},
		"bidi stream: non-OK status with message": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("status:\n  code: Unavailable\n  message: try again"),
			}},
			method: bidiStreamMD,
			stream: &mockStream{ctx: context.Background()},
			expect: "try again",
		},
		"bidi stream: recv error": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage(""),
			}},
			method: bidiStreamMD,
			stream: &mockStream{ctx: context.Background(), recvErr: errors.New("recv failed")},
			expect: "failed to receive messages",
		},
		"bidi stream: send error": {
			mocks: []protocol.Mock{{
				Protocol: "grpc",
				Expect:   yamlutil.RawMessage(""),
				Response: yamlutil.RawMessage("messages:\n  - messageId: '1'"),
			}},
			method: bidiStreamMD,
			stream: &mockStream{ctx: context.Background(), recvMsgs: []any{newMsg(bidiStreamMD)}, sendErr: errors.New("send failed")},
			expect: "failed to send message",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			iter := protocol.NewMockIterator(test.mocks)
			srv := &server{iter: iter}
			handler := srv.streamHandler(svcName, test.method)
			if err := handler(nil, test.stream); err == nil {
				t.Fatal("no error")
			} else if !strings.Contains(err.Error(), test.expect) {
				t.Errorf("expect error %q but got %q", test.expect, err)
			}
		})
	}
}

func TestAccessors(t *testing.T) {
	t.Run("clientStreamRequestAccessor", func(t *testing.T) {
		a := &clientStreamRequestAccessor{}
		if _, ok := a.ExtractByKey(messagesKey); !ok {
			t.Error("expected messages key to be found")
		}
		if _, ok := a.ExtractByKey("other"); ok {
			t.Error("expected other key to not be found")
		}
	})
	t.Run("mockBidiResponseAccessor", func(t *testing.T) {
		a := &mockBidiResponseAccessor{}
		if _, ok := a.ExtractByKey(messagesKey); !ok {
			t.Error("expected messages key to be found")
		}
		if _, ok := a.ExtractByKey("other"); ok {
			t.Error("expected other key to not be found")
		}
	})
	t.Run("mockBidiRequestAccessor", func(t *testing.T) {
		received := make([]*grpcprotocol.ProtoMessageYAMLMarshaler, 0)
		recvDone := false
		a := &mockBidiRequestAccessor{
			mu:       &sync.Mutex{},
			cond:     sync.NewCond(&sync.Mutex{}),
			received: &received,
			recvDone: &recvDone,
		}
		if _, ok := a.ExtractByKey(context.Background(), messagesKey); !ok {
			t.Error("expected messages key to be found")
		}
		if _, ok := a.ExtractByKey(context.Background(), "other"); ok {
			t.Error("expected other key to not be found")
		}
		// Test ExtractByIndex when done and no messages
		recvDone = true
		a.cond.L.Lock()
		a.cond.Broadcast()
		a.cond.L.Unlock()
		if _, ok := a.ExtractByIndex(0); ok {
			t.Error("expected index 0 to not be found when done with no messages")
		}
	})
}
