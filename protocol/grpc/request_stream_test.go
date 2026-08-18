package grpc

import (
	"bytes"
	"testing"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

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
		buf.Close()
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
		buf.Close()
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
