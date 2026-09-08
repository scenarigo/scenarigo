package grpc

import (
	"bytes"
	gocontext "context"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/grpcstream"
	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

type fakeBidiStreamConn struct{}

func (fakeBidiStreamConn) NewInput() (proto.Message, error) { return &testpb.EchoRequest{}, nil }
func (fakeBidiStreamConn) Send(proto.Message) error         { return nil }

//nolint:nilnil // test stub: Recv is never called by abortBidi
func (fakeBidiStreamConn) Recv() (proto.Message, error) { return nil, nil }
func (fakeBidiStreamConn) CloseSend() error             { return nil }
func (fakeBidiStreamConn) HeaderTrailer() (metadata.MD, metadata.MD) {
	return metadata.Pairs("h", "1"), metadata.Pairs("t", "1")
}

func TestAbortBidi(t *testing.T) {
	t.Run("recovers the final status after the stream ended", func(t *testing.T) {
		buf := grpcstream.NewBuffer[proto.Message]()
		buf.Append(&testpb.EchoResponse{MessageId: "1"})
		buf.Close(nil)
		recvCh := make(chan error, 1)
		recvCh <- status.Error(codes.Aborted, "stream aborted")

		result := &streamResult{}
		abortBidi(result, buf, recvCh, fakeBidiStreamConn{})

		if result.sts == nil || result.sts.Code() != codes.Aborted {
			t.Fatalf("expected status Aborted but got %v", result.sts)
		}
		if len(result.messages) != 1 {
			t.Fatalf("expected 1 partial message but got %d", len(result.messages))
		}
		if result.header == nil || result.trailer == nil {
			t.Fatal("expected header/trailer to be captured")
		}
	})
	t.Run("normal close keeps the status nil", func(t *testing.T) {
		buf := grpcstream.NewBuffer[proto.Message]()
		buf.Close(nil)
		recvCh := make(chan error, 1)
		recvCh <- nil

		result := &streamResult{}
		abortBidi(result, buf, recvCh, fakeBidiStreamConn{})

		if result.sts != nil {
			t.Fatalf("expected nil status but got %v", result.sts)
		}
	})
	t.Run("does not block while the stream is still open", func(t *testing.T) {
		buf := grpcstream.NewBuffer[proto.Message]()
		recvCh := make(chan error, 1)

		result := &streamResult{}
		abortBidi(result, buf, recvCh, fakeBidiStreamConn{})

		if result.sts != nil {
			t.Fatalf("expected nil status but got %v", result.sts)
		}
	})
}

func TestBidiResponseAccessor_MarshalYAML(t *testing.T) {
	t.Run("materializes the messages received so far", func(t *testing.T) {
		buf := grpcstream.NewBuffer[proto.Message]()
		buf.Append(&testpb.EchoResponse{MessageId: "1"})
		buf.Append(&testpb.EchoResponse{MessageId: "2"})
		a := &bidiResponseAccessor{buf: buf}

		var b bytes.Buffer
		if err := yaml.NewEncoder(&b, yaml.JSON()).Encode(a); err != nil {
			t.Fatalf("failed to encode: %s", err)
		}
		var got []struct {
			MessageID string `yaml:"messageId"`
		}
		if err := yaml.Unmarshal(b.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode: %s", err)
		}
		if len(got) != 2 || got[0].MessageID != "1" || got[1].MessageID != "2" {
			t.Fatalf("expected messages 1, 2 but got %v", got)
		}
	})
	t.Run("empty buffer materializes as an empty list", func(t *testing.T) {
		a := &bidiResponseAccessor{buf: grpcstream.NewBuffer[proto.Message]()}

		var b bytes.Buffer
		if err := yaml.NewEncoder(&b, yaml.JSON()).Encode(a); err != nil {
			t.Fatalf("failed to encode: %s", err)
		}
		if s := string(bytes.TrimSpace(b.Bytes())); s != "[]" {
			t.Fatalf("expected [] but got %q", s)
		}
	})
}

// blockingBidiStreamConn is a peer that never answers: Recv returns only once
// the stream's own context ends, the way a real one does, with the status gRPC
// synthesises for it. That status does not wrap the context error.
type blockingBidiStreamConn struct {
	ctx gocontext.Context
}

func (c *blockingBidiStreamConn) NewInput() (proto.Message, error) {
	return &testpb.EchoRequest{}, nil
}
func (c *blockingBidiStreamConn) Send(proto.Message) error { return nil }
func (c *blockingBidiStreamConn) Recv() (proto.Message, error) {
	<-c.ctx.Done()
	return nil, status.Error(codes.DeadlineExceeded, "context deadline exceeded")
}
func (c *blockingBidiStreamConn) CloseSend() error { return nil }
func (c *blockingBidiStreamConn) HeaderTrailer() (metadata.MD, metadata.MD) {
	return nil, nil
}

func TestRunBidiStream_AWaitOurOwnDeadlineCutShortIsNotAnAbsence(t *testing.T) {
	// The stream and the template evaluation share the request context, so the
	// deadline that bounds a blocking response reference ends the stream too.
	// Reading the status gRPC synthesises for it as the stream having ended
	// would make the message the reference waited for absent, and ?? would fall
	// back instead of reporting the timeout the guard exists to report.
	reqCtx, cancel := gocontext.WithTimeout(gocontext.Background(), 100*time.Millisecond)
	defer cancel()
	sCtx := context.New(nil).WithRequestContext(reqCtx)
	msgs := []any{
		map[string]any{"messageId": "1"},
		map[string]any{"messageId": `{{response.messages[5].messageId ?? "FALLBACK"}}`},
	}

	_, err := runBidiStream(reqCtx, sCtx, msgs, func(ctx gocontext.Context) (bidiStreamConn, error) {
		return &blockingBidiStreamConn{ctx: ctx}, nil
	})
	if err == nil {
		t.Fatal("the interrupted wait was reported as an absence and absorbed by ??")
	}
	if got, expect := err.Error(), "interrupted while waiting for a streaming response message"; !strings.Contains(got, expect) {
		t.Fatalf("expected an error containing %q but got %q", expect, got)
	}
}
