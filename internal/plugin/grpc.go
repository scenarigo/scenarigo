package plugin

import (
	"context"
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

var (
	typeContext  = reflect.TypeFor[context.Context]()
	typeMessage  = reflect.TypeFor[proto.Message]()
	typeCallOpts = reflect.TypeFor[[]grpc.CallOption]()
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
