package grpc

import (
	"bytes"
	"reflect"

	"github.com/goccy/go-yaml"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
	"github.com/zoncoen/scenarigo/template"
)

// Request represents a request.
type Request struct {
	Client   string      `yaml:"client,omitempty"`
	Method   string      `yaml:"method"`
	Metadata interface{} `yaml:"metadata,omitempty"`
	Body     interface{} `yaml:"body,omitempty"`
}

type response struct {
	Header  metadata.MD     `yaml:"header,omitempty"`
	Trailer metadata.MD     `yaml:"trailer,omitempty"`
	Body    interface{}     `yaml:"body,omitempty"`
	rvalues []reflect.Value `yaml:"-"`
}

// Invoke implements protocol.Invoker interface.
func (r *Request) Invoke(ctx *context.Context) (*context.Context, interface{}, error) {
	if r.Client == "" {
		return ctx, nil, errors.New("gRPC client must be specified")
	}

	x, err := template.Execute(ctx.AddChildPath("request").AddChildPath("client"), r.Client)
	if err != nil {
		return ctx, nil, errors.Errorf("failed to get client: %s", err)
	}

	client := reflect.ValueOf(x)
	var method reflect.Value
	for {
		if !client.IsValid() {
			ctx.AddChildPath("request").AddChildPath("client").ReportYAML()
			return nil, nil, errors.Errorf("client %s is invalid", r.Client)
		}
		method = client.MethodByName(r.Method)
		if method.IsValid() {
			// method found
			break
		}
		switch client.Kind() {
		case reflect.Interface, reflect.Ptr:
			client = client.Elem()
		default:
			ctx.AddChildPath("request").AddChildPath("method").ReportYAML()
			return nil, nil, errors.Errorf("method %s.%s not found", r.Client, r.Method)
		}
	}

	if err := validateMethod(method); err != nil {
		return ctx, nil, errors.Errorf(`"%s.%s" must be "func(context.Context, proto.Message, ...grpc.CallOption) (proto.Message, error): %s"`, r.Client, r.Method, err)
	}

	reqCtx := ctx.RequestContext()
	if r.Metadata != nil {
		x, err := template.Execute(ctx.AddChildPath("request").AddChildPath("metadata"), r.Metadata)
		if err != nil {
			return ctx, nil, errors.Errorf("failed to set metadata: %s", err)
		}
		md, err := reflectutil.ConvertStringsMap(reflect.ValueOf(x))
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to set metadata")
		}

		pairs := []string{}
		for k, vs := range md {
			vs := vs
			for _, v := range vs {
				pairs = append(pairs, k, v)
			}
		}
		reqCtx = metadata.AppendToOutgoingContext(reqCtx, pairs...)
	}

	var in []reflect.Value
	for i := 0; i < method.Type().NumIn(); i++ {
		switch i {
		case 0:
			in = append(in, reflect.ValueOf(reqCtx))
		case 1:
			req := reflect.New(method.Type().In(i).Elem()).Interface()
			if err := buildRequestBody(ctx.AddChildPath("request").AddChildPath("body"), req, r.Body); err != nil {
				return ctx, nil, errors.Errorf("failed to build request body: %s", err)
			}

			ctx = ctx.WithRequest(req)
			reqMD, _ := metadata.FromOutgoingContext(reqCtx)
			if b, err := yaml.Marshal(Request{
				Method:   r.Method,
				Metadata: reqMD,
				Body:     req,
			}); err == nil {
				ctx.Reporter().Logf("request:\n%s", string(b))
			} else {
				ctx.Reporter().Logf("failed to dump request:\n%s", err)
			}

			in = append(in, reflect.ValueOf(req))
		}
	}

	var header, trailer metadata.MD
	in = append(in,
		reflect.ValueOf(grpc.Header(&header)),
		reflect.ValueOf(grpc.Trailer(&trailer)),
	)

	rvalues := method.Call(in)
	body := rvalues[0].Interface()
	resp := response{
		Header:  header,
		Trailer: trailer,
		Body:    body,
		rvalues: rvalues,
	}
	ctx = ctx.WithResponse(resp)
	if b, err := yaml.Marshal(resp); err == nil {
		ctx.Reporter().Logf("response:\n%s", string(b))
	} else {
		ctx.Reporter().Logf("failed to dump response:\n%s", err)
	}

	return ctx, resp, nil
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
	if t := mt.Out(1); !t.Implements(typeError) {
		return errors.Errorf("second return value must be error but got %s", t.String())
	}

	return nil
}

func buildRequestBody(ctx *context.Context, req interface{}, src interface{}) error {
	x, err := template.Execute(ctx, src)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := yaml.NewEncoder(&buf, yaml.JSON()).Encode(x); err != nil {
		return err
	}
	message, ok := req.(proto.Message)
	if ok {
		r := bytes.NewReader(buf.Bytes())
		if err := jsonpb.Unmarshal(r, message); err != nil {
			return err
		}
	}
	return nil
}
