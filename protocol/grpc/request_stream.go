package grpc

import (
	gocontext "context"
	stderrors "errors"
	"fmt"
	"io"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/grpcstream"
)

// The streaming control flow (EOF tolerance, partial-result harvesting, the
// background receiver goroutine, and final-status recovery) is shared between
// the proto/reflection client and the custom client plugins: each client only
// implements the per-kind stream connection interface below, and the run*
// functions own the flow. The interfaces are split per RPC kind so that each
// one requires exactly the operations that kind of stream really has.

// serverStreamConn is the receive side of an opened server-streaming call.
// The request message is sent as part of opening the stream.
type serverStreamConn interface {
	Recv() (proto.Message, error)
	HeaderTrailer() (metadata.MD, metadata.MD)
}

// clientStreamConn is the send side of an opened client-streaming call.
type clientStreamConn interface {
	Send(proto.Message) error
	CloseAndRecv() (proto.Message, error)
	HeaderTrailer() (metadata.MD, metadata.MD)
}

// bidiStreamConn is an opened bidirectional-streaming call.
type bidiStreamConn interface {
	NewInput() (proto.Message, error)
	Send(proto.Message) error
	Recv() (proto.Message, error)
	CloseSend() error
	HeaderTrailer() (metadata.MD, metadata.MD)
}

// runServerStream receives all streamed responses. On a mid-stream error it
// keeps the messages received so far so they can be dumped for debugging.
func runServerStream(open func() (serverStreamConn, error)) (*streamResult, error) {
	stream, err := open()
	if err != nil {
		return openFailure(&streamResult{}, err)
	}

	var msgs []proto.Message
	for {
		msg, err := stream.Recv()
		if err != nil {
			if stderrors.Is(err, io.EOF) {
				break
			}
			header, trailer := stream.HeaderTrailer()
			return &streamResult{messages: msgs, header: header, trailer: trailer, sts: status.Convert(err)}, nil
		}
		msgs = append(msgs, msg)
	}

	header, trailer := stream.HeaderTrailer()
	return &streamResult{messages: msgs, header: header, trailer: trailer}, nil
}

// runClientStream sends the pre-built request messages and receives the single
// response.
func runClientStream(open func() (clientStreamConn, error), msgs []proto.Message) (*streamResult, error) {
	stream, err := open()
	if err != nil {
		return openFailure(&streamResult{}, err)
	}

	for _, msg := range msgs {
		if err := stream.Send(msg); err != nil {
			// Send returns io.EOF when the server terminates the stream;
			// the actual status is retrieved by CloseAndRecv below.
			if stderrors.Is(err, io.EOF) {
				break
			}
			return &streamResult{sts: status.Convert(err)}, nil
		}
	}

	respMsg, err := stream.CloseAndRecv()
	if err != nil {
		header, trailer := stream.HeaderTrailer()
		return &streamResult{header: header, trailer: trailer, sts: status.Convert(err)}, nil
	}

	header, trailer := stream.HeaderTrailer()
	return &streamResult{message: respMsg, header: header, trailer: trailer}, nil
}

// runBidiStream sends the request messages sequentially, evaluating each
// template as it goes so that a message can reference already-received
// responses (response.messages[N], blocking) and already-sent requests
// (request.messages[N]).
func runBidiStream(ctx gocontext.Context, sCtx *context.Context, msgs []any, open func(gocontext.Context) (bidiStreamConn, error)) (*streamResult, error) {
	result := &streamResult{}
	// Cancel the stream when we return so the background receiver goroutine and
	// the server RPC are released even if we bail out mid-stream (e.g. when the
	// deadlock guard fires while evaluating a response reference).
	streamCtx, cancelStream := gocontext.WithCancel(ctx)
	defer cancelStream()
	stream, err := open(streamCtx)
	if err != nil {
		return openFailure(result, err)
	}

	// Accumulate responses in the background. Blocking response references
	// (response.messages[N]) wait on the buffer, bounded by the template
	// evaluation context, so a deadlocked scenario fails instead of hanging.
	buf := grpcstream.NewBuffer[proto.Message]()
	recvCh := make(chan error, 1)
	go func() {
		for {
			out, err := stream.Recv()
			if err != nil {
				buf.Close()
				if stderrors.Is(err, io.EOF) {
					err = nil
				}
				recvCh <- err
				return
			}
			buf.Append(out)
		}
	}()

	// Set up a response accessor that blocks until the Nth response is available
	bidiResp := &bidiResponseAccessor{buf: buf}
	sCtx = sCtx.WithResponse(bidiResp)

	// Set up a request accessor for referencing already-sent messages
	bidiReq := &requestMessagesAccessor{}
	sCtx = sCtx.WithRequest(bidiReq)

	// Send messages sequentially, evaluating templates as we go. Each
	// evaluation is bounded by the caller's deadline, or by the deadlock guard
	// when none is set, so a blocking response reference that can never be
	// satisfied fails instead of hanging. Putting the guard on the context lets
	// the template engine report an interrupted wait as an error instead of
	// treating it as an undefined value.
	for i, m := range msgs {
		evalCtx := sCtx
		cancelEval := gocontext.CancelFunc(func() {})
		if _, ok := sCtx.RequestContext().Deadline(); !ok {
			c, cancel := gocontext.WithTimeout(sCtx.RequestContext(), grpcstream.DefaultMessageWaitTimeout)
			evalCtx, cancelEval = sCtx.WithRequestContext(c), cancel
		}
		x, err := evalCtx.ExecuteTemplate(m)
		evalErr := evalCtx.RequestContext().Err()
		cancelEval()
		if err != nil {
			abortBidi(result, buf, recvCh, stream)
			// Distinguish the interruption causes: only an expired deadline
			// suggests a deadlock, while a cancellation is an external abort.
			switch {
			case stderrors.Is(evalErr, gocontext.DeadlineExceeded):
				return result, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "interrupted while waiting for a streaming response message (possible deadlock or timeout)")
			case evalErr != nil:
				return result, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "canceled while waiting for a streaming response message")
			}
			return result, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to execute template")
		}
		in, err := stream.NewInput()
		if err != nil {
			abortBidi(result, buf, recvCh, stream)
			return result, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to build request message")
		}
		if x != nil {
			if err := ConvertToProto(x, in); err != nil {
				abortBidi(result, buf, recvCh, stream)
				return result, errors.WrapPathf(err, fmt.Sprintf("messages[%d]", i), "failed to build request message")
			}
		}
		// Record the message before sending so that failed attempts also appear in the dump.
		bidiReq.sent = append(bidiReq.sent, in)
		result.sent = bidiReq.sent
		if err := stream.Send(in); err != nil {
			// Send returns io.EOF when the server terminates the stream;
			// the actual status is reported by the receiver goroutine.
			if stderrors.Is(err, io.EOF) {
				break
			}
			result.messages = buf.Snapshot()
			result.sts = status.Convert(err)
			return result, nil
		}
	}

	if err := stream.CloseSend(); err != nil {
		result.messages = buf.Snapshot()
		result.sts = status.Convert(err)
		return result, nil
	}

	// Wait for all responses
	recvErr := <-recvCh
	result.messages = buf.Snapshot()
	result.header, result.trailer = stream.HeaderTrailer()
	if recvErr != nil {
		result.sts = status.Convert(recvErr)
	}
	return result, nil
}

// openFailure records an open() failure. A gRPC status error — including
// transport failures such as Unavailable — is observable scenario state and is
// recorded as the result status, matching the unary behavior; any other error
// is a client-side bug (e.g. a plugin returning a nil stream) and is escalated
// as a hard error so that a scenario cannot accidentally assert it away with
// an expect.status.code.
func openFailure(result *streamResult, err error) (*streamResult, error) {
	if s, ok := status.FromError(err); ok {
		result.sts = s
		return result, nil
	}
	return result, err
}

// abortBidi records the partial results for the dump when runBidiStream bails
// out mid-stream. If the stream has already ended, the receiver goroutine
// reports the final status to recvCh right after closing the buffer (the
// channel is buffered, so the send always completes); recover it so the dump
// shows why the stream ended rather than only the local failure.
func abortBidi(result *streamResult, buf *grpcstream.Buffer[proto.Message], recvCh <-chan error, stream bidiStreamConn) {
	result.messages = buf.Snapshot()
	if buf.Done() {
		if recvErr := <-recvCh; recvErr != nil {
			result.sts = status.Convert(recvErr)
		}
		result.header, result.trailer = stream.HeaderTrailer()
	}
}

// requestMessagesAccessor provides access to already-built request messages.
type requestMessagesAccessor struct {
	sent []proto.Message
}

// ExtractByKey implements query.KeyExtractor interface.
func (a *requestMessagesAccessor) ExtractByKey(key string) (any, bool) {
	if key == "messages" {
		msgs := make([]*ProtoMessageYAMLMarshaler, len(a.sent))
		for i, m := range a.sent {
			msgs[i] = &ProtoMessageYAMLMarshaler{m}
		}
		return msgs, true
	}
	return nil, false
}

// bidiResponseAccessor provides access to streaming responses with blocking semantics.
// When accessing messages[N], it blocks until the Nth response has been received,
// the stream is closed, or the wait is canceled (deadlock/timeout guard).
type bidiResponseAccessor struct {
	buf *grpcstream.Buffer[proto.Message]
}

// ExtractByKey implements query.KeyExtractorContext interface.
func (a *bidiResponseAccessor) ExtractByKey(_ gocontext.Context, key string) (any, bool) {
	if key == "messages" {
		return a, true
	}
	return nil, false
}

// ExtractByIndex implements query.IndexExtractorContext interface. It blocks
// until the Nth response has been received, bounded by ctx.
func (a *bidiResponseAccessor) ExtractByIndex(ctx gocontext.Context, i int) (any, bool) {
	msg, ok := a.buf.At(ctx, i)
	if !ok {
		return nil, false
	}
	return &ProtoMessageYAMLMarshaler{msg}, true
}
