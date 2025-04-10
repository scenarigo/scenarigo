package grpc

import (
	"bytes"
	gocontext "context"
	"fmt"
	"reflect"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

type customServiceClient struct {
	r      *Request
	method reflect.Value
}

func newCustomServiceClient(r *Request, v reflect.Value) (*customServiceClient, error) {
	var method reflect.Value
	for {
		if !v.IsValid() {
			return nil, errors.ErrorPathf("client", "client %q is invalid", r.Client)
		}
		method = v.MethodByName(r.Method)
		if method.IsValid() {
			// method found
			break
		}
		switch v.Kind() {
		case reflect.Interface, reflect.Ptr:
			v = v.Elem()
		default:
			return nil, errors.ErrorPathf("method", `method "%s.%s" not found`, r.Client, r.Method)
		}
	}

	if err := validateMethod(method); err != nil {
		return nil, errors.ErrorPathf("method", `"%s.%s" must be "func(context.Context, proto.Message, ...grpc.CallOption) (proto.Message, error): %s"`, r.Client, r.Method, err)
	}

	return &customServiceClient{
		r:      r,
		method: method,
	}, nil
}

func (client *customServiceClient) buildRequestMessage(ctx *context.Context) (proto.Message, error) {
	req := reflect.New(client.method.Type().In(1).Elem()).Interface()
	if err := buildRequestMsg(ctx, req, client.r.Message); err != nil {
		return nil, errors.WrapPathf(err, "message", "failed to build request message")
	}
	reqMsg, ok := req.(proto.Message)
	if !ok {
		return nil, errors.ErrorPathf("client", "failed to build request message: second argument must be proto.Message but %T", req)
	}
	return reqMsg, nil
}

func (client *customServiceClient) invoke(ctx gocontext.Context, reqMsg proto.Message, opts ...grpc.CallOption) (proto.Message, *status.Status, error) {
	in := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(reqMsg),
	}
	for _, o := range opts {
		in = append(in, reflect.ValueOf(o))
	}

	rvalues := client.method.Call(in)
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

func validateMethod(method reflect.Value) error {
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

func buildRequestMsg(ctx *context.Context, req any, src any) error {
	x, err := ctx.ExecuteTemplate(src)
	if err != nil {
		return err
	}
	if x == nil {
		return nil
	}
	msg, ok := req.(proto.Message)
	if !ok {
		return fmt.Errorf("expect proto.Message but got %T", req)
	}
	return ConvertToProto(x, msg)
}

func ConvertToProto(v any, msg proto.Message) error {
	var buf bytes.Buffer
	if err := yaml.NewEncoder(&buf, yaml.JSON()).Encode(v); err != nil {
		return err
	}
	if err := protojson.Unmarshal(buf.Bytes(), msg); err != nil {
		return err
	}
	return nil
}
