package plugin

import (
	"context"
	"io"
	"reflect"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

type CustomGRPCClient interface {
	ExistsMethod(method string) bool
	BuildRequestMessage(method string, params []byte) (proto.Message, error)
	Invoke(ctx context.Context, method string, req proto.Message) (proto.Message, *status.Status, error)
}

// GRPCMethodType represents the type of a gRPC method.
type GRPCMethodType int

const (
	GRPCMethodUnary GRPCMethodType = iota
	GRPCMethodServerStream
	GRPCMethodClientStream
	GRPCMethodBidiStream
)

var (
	typeContext      = reflect.TypeFor[context.Context]()
	typeMessage      = reflect.TypeFor[proto.Message]()
	typeCallOpts     = reflect.TypeFor[[]grpc.CallOption]()
	typeClientStream = reflect.TypeFor[grpc.ClientStream]()
)

func ValidateGRPCMethod(method reflect.Value) error {
	if !method.IsValid() {
		return errors.New("invalid")
	}
	if method.Kind() != reflect.Func {
		return errors.New("not function")
	}
	if method.IsNil() {
		return errors.New("method is nil")
	}

	mt := method.Type()
	if n := mt.NumIn(); n != 3 {
		return errors.Errorf("number of arguments must be 3 but got %d", n)
	}
	if t := mt.In(0); !t.Implements(typeContext) {
		return errors.Errorf("first argument must be context.Context but got %s", t.String())
	}
	if t := mt.In(1); !t.Implements(typeMessage) {
		return errors.Errorf("second argument must be proto.Message but got %s", t.String())
	}
	if t := mt.In(2); t != typeCallOpts {
		return errors.Errorf("third argument must be []grpc.CallOption but got %s", t.String())
	}
	if n := mt.NumOut(); n != 2 {
		return errors.Errorf("number of return values must be 2 but got %d", n)
	}
	if t := mt.Out(0); !t.Implements(typeMessage) {
		return errors.Errorf("first return value must be proto.Message but got %s", t.String())
	}
	if t := mt.Out(1); !t.Implements(reflectutil.TypeError) {
		return errors.Errorf("second return value must be error but got %s", t.String())
	}

	return nil
}

func GRPCInvoke(ctx context.Context, method reflect.Value, reqMsg proto.Message, opts ...grpc.CallOption) (proto.Message, *status.Status, error) {
	in := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(reqMsg),
	}
	for _, o := range opts {
		in = append(in, reflect.ValueOf(o))
	}

	rvalues := method.Call(in)
	if len(rvalues) != 2 {
		return nil, nil, errors.Errorf("expected return value length of method call is 2 but %d", len(rvalues))
	}
	if !rvalues[0].IsValid() {
		return nil, nil, errors.New("first return value is invalid")
	}
	respMsg, ok := rvalues[0].Interface().(proto.Message)
	if !ok {
		if !rvalues[0].IsNil() {
			return nil, nil, errors.Errorf("expected first return value is proto.Message but %T", rvalues[0].Interface())
		}
	}
	if !rvalues[1].IsValid() {
		return nil, nil, errors.New("second return value is invalid")
	}
	callErr, ok := rvalues[1].Interface().(error)
	if !ok {
		if !rvalues[1].IsNil() {
			return nil, nil, errors.Errorf("expected second return value is error but %T", rvalues[1].Interface())
		}
	}
	var sts *status.Status
	if ok {
		sts, ok = status.FromError(callErr)
		if !ok {
			return nil, nil, errors.Errorf(`expected gRPC status error but got %T: "%s"`, callErr, callErr.Error())
		}
	}

	return respMsg, sts, nil
}

// DetectGRPCMethodType determines the gRPC method type from a reflected method value.
//
// Signatures:
//   - Unary:         func(ctx, *Req, ...CallOption) (*Resp, error) — 3 args, Out(0) implements proto.Message
//   - Server Stream: func(ctx, *Req, ...CallOption) (StreamClient, error) — 3 args, Out(0) implements grpc.ClientStream
//   - Client Stream: func(ctx, ...CallOption) (StreamClient, error) — 2 args, has Send + CloseAndRecv
//   - Bidi Stream:   func(ctx, ...CallOption) (StreamClient, error) — 2 args, has Send + Recv
func DetectGRPCMethodType(method reflect.Value) GRPCMethodType {
	mt := method.Type()
	numIn := mt.NumIn()

	if numIn == 3 {
		// Unary or Server Stream — check if return type implements proto.Message (unary) or grpc.ClientStream (server stream)
		if mt.NumOut() >= 1 && mt.Out(0).Implements(typeClientStream) {
			return GRPCMethodServerStream
		}
		return GRPCMethodUnary
	}

	if numIn == 2 {
		// Client Stream or Bidi Stream — check stream return type for Recv method
		if mt.NumOut() >= 1 {
			streamType := mt.Out(0)
			hasRecv := false
			for i := range streamType.NumMethod() {
				if streamType.Method(i).Name == "Recv" {
					hasRecv = true
					break
				}
			}
			if hasRecv {
				return GRPCMethodBidiStream
			}
		}
		return GRPCMethodClientStream
	}

	return GRPCMethodUnary
}

// ValidateGRPCStreamingMethod validates a streaming gRPC method signature.
func ValidateGRPCStreamingMethod(method reflect.Value, methodType GRPCMethodType) error {
	if !method.IsValid() {
		return errors.New("invalid")
	}
	if method.Kind() != reflect.Func {
		return errors.New("not function")
	}
	if method.IsNil() {
		return errors.New("method is nil")
	}

	mt := method.Type()

	switch methodType {
	case GRPCMethodServerStream:
		if n := mt.NumIn(); n != 3 {
			return errors.Errorf("number of arguments must be 3 but got %d", n)
		}
		if t := mt.In(0); !t.Implements(typeContext) {
			return errors.Errorf("first argument must be context.Context but got %s", t.String())
		}
		if t := mt.In(1); !t.Implements(typeMessage) {
			return errors.Errorf("second argument must be proto.Message but got %s", t.String())
		}
		if t := mt.In(2); t != typeCallOpts {
			return errors.Errorf("third argument must be []grpc.CallOption but got %s", t.String())
		}
	case GRPCMethodClientStream, GRPCMethodBidiStream:
		if n := mt.NumIn(); n != 2 {
			return errors.Errorf("number of arguments must be 2 but got %d", n)
		}
		if t := mt.In(0); !t.Implements(typeContext) {
			return errors.Errorf("first argument must be context.Context but got %s", t.String())
		}
		if t := mt.In(1); t != typeCallOpts {
			return errors.Errorf("second argument must be []grpc.CallOption but got %s", t.String())
		}
	default:
		return errors.Errorf("unexpected method type for streaming validation: %d", methodType)
	}

	if n := mt.NumOut(); n != 2 {
		return errors.Errorf("number of return values must be 2 but got %d", n)
	}
	if t := mt.Out(0); !t.Implements(typeClientStream) {
		return errors.Errorf("first return value must implement grpc.ClientStream but got %s", t.String())
	}
	if t := mt.Out(1); !t.Implements(reflectutil.TypeError) {
		return errors.Errorf("second return value must be error but got %s", t.String())
	}

	return nil
}

// GRPCInvokeServerStream invokes a server streaming method via reflection.
// Returns the stream as a reflect.Value.
func GRPCInvokeServerStream(ctx context.Context, method reflect.Value, reqMsg proto.Message, opts ...grpc.CallOption) (reflect.Value, error) {
	in := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(reqMsg),
	}
	for _, o := range opts {
		in = append(in, reflect.ValueOf(o))
	}

	rvalues := method.Call(in)
	if len(rvalues) != 2 {
		return reflect.Value{}, errors.Errorf("expected return value length of method call is 2 but %d", len(rvalues))
	}
	if !rvalues[1].IsNil() {
		if callErr, ok := rvalues[1].Interface().(error); ok {
			return reflect.Value{}, callErr
		}
	}
	return rvalues[0], nil
}

// GRPCInvokeStream invokes a client or bidi streaming method via reflection (no request message argument).
// Returns the stream as a reflect.Value.
func GRPCInvokeStream(ctx context.Context, method reflect.Value, opts ...grpc.CallOption) (reflect.Value, error) {
	in := []reflect.Value{
		reflect.ValueOf(ctx),
	}
	for _, o := range opts {
		in = append(in, reflect.ValueOf(o))
	}

	rvalues := method.Call(in)
	if len(rvalues) != 2 {
		return reflect.Value{}, errors.Errorf("expected return value length of method call is 2 but %d", len(rvalues))
	}
	if !rvalues[1].IsNil() {
		if callErr, ok := rvalues[1].Interface().(error); ok {
			return reflect.Value{}, callErr
		}
	}
	return rvalues[0], nil
}

// GRPCStreamSend calls the Send method on a stream via reflection.
func GRPCStreamSend(stream reflect.Value, msg proto.Message) error {
	sendMethod := stream.MethodByName("Send")
	if !sendMethod.IsValid() {
		return errors.New("stream does not have Send method")
	}
	rvalues := sendMethod.Call([]reflect.Value{reflect.ValueOf(msg)})
	if len(rvalues) != 1 {
		return errors.Errorf("expected 1 return value from Send but got %d", len(rvalues))
	}
	if !rvalues[0].IsNil() {
		if err, ok := rvalues[0].Interface().(error); ok {
			return err
		}
	}
	return nil
}

// GRPCStreamRecv calls the Recv method on a stream via reflection.
func GRPCStreamRecv(stream reflect.Value) (proto.Message, error) {
	recvMethod := stream.MethodByName("Recv")
	if !recvMethod.IsValid() {
		return nil, errors.New("stream does not have Recv method")
	}
	rvalues := recvMethod.Call(nil)
	if len(rvalues) != 2 {
		return nil, errors.Errorf("expected 2 return values from Recv but got %d", len(rvalues))
	}
	if !rvalues[1].IsNil() {
		if err, ok := rvalues[1].Interface().(error); ok {
			return nil, err
		}
	}
	if rvalues[0].IsNil() {
		return nil, io.EOF
	}
	msg, ok := rvalues[0].Interface().(proto.Message)
	if !ok {
		return nil, errors.Errorf("expected proto.Message from Recv but got %T", rvalues[0].Interface())
	}
	return msg, nil
}

// GRPCStreamCloseAndRecv calls the CloseAndRecv method on a client stream via reflection.
func GRPCStreamCloseAndRecv(stream reflect.Value) (proto.Message, error) {
	m := stream.MethodByName("CloseAndRecv")
	if !m.IsValid() {
		return nil, errors.New("stream does not have CloseAndRecv method")
	}
	rvalues := m.Call(nil)
	if len(rvalues) != 2 {
		return nil, errors.Errorf("expected 2 return values from CloseAndRecv but got %d", len(rvalues))
	}
	if !rvalues[1].IsNil() {
		if err, ok := rvalues[1].Interface().(error); ok {
			return nil, err
		}
	}
	if rvalues[0].IsNil() {
		return nil, errors.New("CloseAndRecv returned nil response")
	}
	msg, ok := rvalues[0].Interface().(proto.Message)
	if !ok {
		return nil, errors.Errorf("expected proto.Message from CloseAndRecv but got %T", rvalues[0].Interface())
	}
	return msg, nil
}

// GRPCStreamCloseSend calls the CloseSend method on a stream via reflection.
func GRPCStreamCloseSend(stream reflect.Value) error {
	m := stream.MethodByName("CloseSend")
	if !m.IsValid() {
		return errors.New("stream does not have CloseSend method")
	}
	rvalues := m.Call(nil)
	if len(rvalues) != 1 {
		return errors.Errorf("expected 1 return value from CloseSend but got %d", len(rvalues))
	}
	if !rvalues[0].IsNil() {
		if err, ok := rvalues[0].Interface().(error); ok {
			return err
		}
	}
	return nil
}

// GRPCStreamRequestType returns the request message type for a streaming method.
// For server stream: method.Type().In(1).Elem() (same as unary).
// For client/bidi stream: derived from the Send method's argument type.
func GRPCStreamRequestType(method reflect.Value, methodType GRPCMethodType) (reflect.Type, error) {
	mt := method.Type()
	switch methodType {
	case GRPCMethodServerStream:
		return mt.In(1).Elem(), nil
	case GRPCMethodClientStream, GRPCMethodBidiStream:
		streamType := mt.Out(0)
		sendMethod, ok := streamType.MethodByName("Send")
		if !ok {
			return nil, errors.New("stream type does not have Send method")
		}
		// Send method signature: func(stream, *Req) error — In(0) is *Req
		if sendMethod.Type.NumIn() < 1 {
			return nil, errors.New("Send method has no arguments")
		}
		return sendMethod.Type.In(0).Elem(), nil
	default:
		return mt.In(1).Elem(), nil
	}
}

// GRPCStreamAsClientStream extracts the grpc.ClientStream interface from a stream reflect.Value.
func GRPCStreamAsClientStream(stream reflect.Value) (grpc.ClientStream, bool) {
	if !stream.IsValid() || stream.IsNil() {
		return nil, false
	}
	cs, ok := stream.Interface().(grpc.ClientStream)
	return cs, ok
}
