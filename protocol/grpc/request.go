package grpc

import (
	gocontext "context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"dario.cat/mergo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/goccy/go-yaml"
	"github.com/zoncoen/query-go"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/filepathutil"
	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/internal/reflectutil"
	"github.com/scenarigo/scenarigo/internal/yamlutil"
)

var tlsVers = map[string]uint16{
	tls.VersionName(tls.VersionTLS10): tls.VersionTLS10,
	tls.VersionName(tls.VersionTLS11): tls.VersionTLS11,
	tls.VersionName(tls.VersionTLS12): tls.VersionTLS12,
	tls.VersionName(tls.VersionTLS13): tls.VersionTLS13,
	tls.VersionName(tls.VersionSSL30): tls.VersionSSL30,
}

// Request represents a request.
type Request struct {
	Client   string          `yaml:"client,omitempty"`
	Target   string          `yaml:"target,omitempty"`
	Service  string          `yaml:"service,omitempty"`
	Method   string          `yaml:"method,omitempty"`
	Metadata interface{}     `yaml:"metadata,omitempty"`
	Message  interface{}     `yaml:"message,omitempty"`
	Options  *RequestOptions `yaml:"options,omitempty"`

	// for backward compatibility
	Body interface{} `yaml:"body,omitempty"`
}

// RequestOptions represents request options.
type RequestOptions struct {
	Reflection *ReflectionOption `yaml:"reflection,omitempty"`
	Proto      *ProtoOption      `yaml:"proto,omitempty"`
	Auth       *AuthOption       `yaml:"auth,omitempty"`
}

// ReflectionOption represents a gRPC reflection service option.
type ReflectionOption struct {
	Enabled bool `yaml:"enabled,omitempty"`
}

func (o *ReflectionOption) IsEnabled() bool {
	if o != nil {
		return o.Enabled
	}
	return false
}

// ProtoOption represents a protocol buffers option.
type ProtoOption struct {
	Imports []string `yaml:"imports,omitempty"`
	Files   []string `yaml:"files,omitempty"`
}

// AuthOption represents a authentication option.
type AuthOption struct {
	Insecure *bool      `json:"insecure,omitempty" yaml:"insecure,omitempty"`
	TLS      *TLSOption `json:"tls,omitempty"      yaml:"tls,omitempty"`
}

// Credentials returns a credentials for transport security.
func (o *AuthOption) Credentials() (credentials.TransportCredentials, error) {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if o == nil {
		return credentials.NewTLS(cfg), nil
	}
	if o.Insecure != nil && *o.Insecure {
		return insecure.NewCredentials(), nil
	}
	if o.TLS == nil {
		return credentials.NewTLS(cfg), nil
	}
	if o.TLS.MinVersion != "" {
		v, ok := tlsVers[o.TLS.MinVersion]
		if !ok {
			return nil, errors.ErrorPathf("tls.minVersion", "invalid minimum TLS version %s", o.TLS.MinVersion)
		}
		cfg.MinVersion = v
	}
	if o.TLS.MaxVersion != "" {
		v, ok := tlsVers[o.TLS.MaxVersion]
		if !ok {
			return nil, errors.ErrorPathf("tls.maxVersion", "invalid maximum TLS version %s", o.TLS.MaxVersion)
		}
		cfg.MaxVersion = v
	}
	if o.TLS.Certificate != "" {
		b, err := os.ReadFile(o.TLS.Certificate)
		if err != nil {
			return nil, errors.WrapPath(err, "tls.certificate", "failed to read certificate")
		}
		cp := x509.NewCertPool()
		if !cp.AppendCertsFromPEM(b) {
			return nil, errors.WrapPath(err, "tls.certificate", "failed to append certificate")
		}
		cfg.RootCAs = cp
	}
	if o.TLS.Skip {
		cfg.InsecureSkipVerify = true
	}
	return credentials.NewTLS(cfg), nil
}

// TLSOption represents a TLS option.
type TLSOption struct {
	// MinVersion contains the minimum TLS version that is acceptable.
	// By default, TLS 1.2 is currently used as the minimum.
	MinVersion string `json:"minVersion,omitempty" yaml:"minVersion,omitempty"`

	// MaxVersion contains the maximum TLS version that is acceptable.
	// By default, TLS 1.3 is currently used as the maximum.
	MaxVersion string `json:"maxVersion,omitempty" yaml:"maxVersion,omitempty"`

	Certificate string `json:"certificate,omitempty" yaml:"certificate,omitempty"`
	Skip        bool   `json:"skip,omitempty"        yaml:"skip,omitempty"`
}

// RequestExtractor represents a request dump.
type RequestExtractor request

// ExtractByKey implements query.KeyExtractor interface.
func (r RequestExtractor) ExtractByKey(key string) (interface{}, bool) {
	q := queryutil.New().Key(key)
	if v, err := q.Extract(request(r)); err == nil {
		return v, true
	}
	// for backward compatibility
	if v, err := q.Extract(r.Message); err == nil {
		return v, true
	}
	return nil, false
}

type request struct {
	Method   string                     `yaml:"method,omitempty"`
	Metadata any                        `yaml:"metadata,omitempty"`
	Message  *ProtoMessageYAMLMarshaler `yaml:"message,omitempty"`
}

type ProtoMessageYAMLMarshaler struct {
	proto.Message `yaml:",inline"`
}

// MarshalYAML implements yaml.BytesMarshalerContext interface.
func (m *ProtoMessageYAMLMarshaler) MarshalYAML(_ gocontext.Context) ([]byte, error) {
	jb, err := protojson.Marshal(m.Message)
	if err != nil {
		return nil, err
	}
	yb, err := yaml.JSONToYAML(jb)
	return yb, err
}

type response struct {
	Status  *responseStatus            `yaml:"status,omitempty"`
	Header  *yamlutil.MDMarshaler      `yaml:"header,omitempty"`
	Trailer *yamlutil.MDMarshaler      `yaml:"trailer,omitempty"`
	Message *ProtoMessageYAMLMarshaler `yaml:"message,omitempty"`
}

type responseStatus struct {
	*status.Status
}

func (s *responseStatus) Code() codes.Code {
	if s == nil {
		return codes.OK
	}
	return s.Status.Code()
}

func (s *responseStatus) Message() string {
	if s == nil {
		return ""
	}
	return s.Status.Message()
}

func (s *responseStatus) Details() []any {
	if s == nil {
		return nil
	}
	return s.Status.Details()
}

func (s *responseStatus) Marshaler() *responseStatusMarshaler {
	v := &responseStatusMarshaler{
		Code:    s.Code().String(),
		Message: s.Message(),
	}
	details := s.Details()
	if l := len(details); l > 0 {
		m := make(yaml.MapSlice, l)
		for i, d := range details {
			item := yaml.MapItem{
				Key:   "",
				Value: d,
			}
			if msg, ok := d.(proto.Message); ok {
				item.Key = string(proto.MessageName(msg))
			} else {
				item.Key = fmt.Sprintf("%T (not proto.Message)", d)
			}
			m[i] = item
		}
		v.Details = m
	}
	return v
}

type responseStatusMarshaler struct {
	Code    string        `yaml:"code,omitempty"`
	Message string        `yaml:"message,omitempty"`
	Details yaml.MapSlice `yaml:"details,omitempty"`
}

// MarshalYAML implements yaml.BytesMarshalerContext interface.
func (s *responseStatus) MarshalYAML(ctx gocontext.Context) ([]byte, error) {
	return yaml.MarshalContext(ctx, s.Marshaler())
}

// ExtractByKey implements query.KeyExtractorContext interface.
func (s *responseStatus) ExtractByKey(ctx gocontext.Context, key string) (interface{}, bool) {
	var opts []query.Option
	if query.IsCaseInsensitive(ctx) {
		opts = append(opts, query.CaseInsensitive())
	}
	q := queryutil.New(opts...).Key(key)
	if got, err := q.Extract(s.Marshaler()); err == nil {
		return got, true
	}
	return nil, false
}

// ResponseExtractor represents a response dump.
type ResponseExtractor response

// ExtractByKey implements query.KeyExtractorContext interface.
func (r ResponseExtractor) ExtractByKey(ctx gocontext.Context, key string) (interface{}, bool) {
	var opts []query.Option
	if query.IsCaseInsensitive(ctx) {
		opts = append(opts, query.CaseInsensitive())
	}
	q := queryutil.New(opts...).Key(key)
	if v, err := q.Extract(response(r)); err == nil {
		return v, true
	}
	// for backward compatibility
	if v, err := q.Extract(r.Message); err == nil {
		return v, true
	}
	return nil, false
}

const (
	indentNum = 2
)

func (r *Request) addIndent(s string, indentNum int) string {
	indent := strings.Repeat(" ", indentNum)
	lines := []string{}
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			lines = append(lines, line)
		} else {
			lines = append(lines, fmt.Sprintf("%s%s", indent, line))
		}
	}
	return strings.Join(lines, "\n")
}

// Invoke implements protocol.Invoker interface.
func (r *Request) Invoke(ctx *context.Context) (*context.Context, interface{}, error) {
	opts := &RequestOptions{}
	if r.Options != nil {
		if err := mergo.Merge(opts, r.Options); err != nil {
			return ctx, nil, errors.WrapPath(err, "options", "failed to apply options")
		}
	}
	if pOpt := grpcProtocol.getOption(); pOpt != nil && pOpt.Request != nil {
		if err := mergo.Merge(opts, pOpt.Request, mergo.WithoutDereference); err != nil {
			return ctx, nil, errors.WrapPath(err, "options", "failed to apply options")
		}
	}
	opts, err := context.ExecuteTemplate(ctx, opts)
	if err != nil {
		return ctx, nil, errors.WrapPath(err, "options", "failed to execute template")
	}
	if opts.Proto != nil {
		dir := filepath.Dir(ctx.ScenarioFilepath())
		for i, p := range opts.Proto.Imports {
			opts.Proto.Imports[i] = filepathutil.From(dir, p)
		}
		// If import paths present and not empty, then all file paths to find are assumed to be relative to one of these paths.
		if len(opts.Proto.Imports) == 0 {
			for i, p := range opts.Proto.Files {
				opts.Proto.Files[i] = filepathutil.From(dir, p)
			}
		}
	}

	client, err := r.buildClient(ctx, opts)
	if err != nil {
		return ctx, nil, err
	}
	ctx, err = r.appendMetadata(ctx)
	if err != nil {
		return ctx, nil, err
	}
	reqMsg, err := client.buildRequestMessage(ctx)
	if err != nil {
		return ctx, nil, err
	}
	ctx = r.dumpRequest(ctx, reqMsg)

	var header, trailer metadata.MD
	callOpts := []grpc.CallOption{
		grpc.Header(&header),
		grpc.Trailer(&trailer),
	}
	respMsg, sts, err := client.invoke(ctx.RequestContext(), reqMsg, callOpts...)
	if err != nil {
		return ctx, nil, err
	}
	resp := &response{
		Status: &responseStatus{
			status.New(codes.OK, ""),
		},
		Message: &ProtoMessageYAMLMarshaler{respMsg},
	}
	if sts != nil {
		resp.Status = &responseStatus{sts}
	}
	if len(header) > 0 {
		resp.Header = yamlutil.NewMDMarshaler(header)
	}
	if len(trailer) > 0 {
		resp.Trailer = yamlutil.NewMDMarshaler(trailer)
	}
	ctx = ctx.WithResponse((*ResponseExtractor)(resp))
	if b, err := yaml.Marshal(resp); err == nil {
		ctx.Reporter().Logf("response:\n%s", r.addIndent(string(b), indentNum))
	} else {
		ctx.Reporter().Logf("failed to dump response:\n%s", err)
	}

	return ctx, resp, nil
}

type serviceClient interface {
	buildRequestMessage(*context.Context) (proto.Message, error)
	invoke(gocontext.Context, proto.Message, ...grpc.CallOption) (proto.Message, *status.Status, error)
}

func (r *Request) buildClient(ctx *context.Context, opts *RequestOptions) (serviceClient, error) {
	if r.Client != "" {
		x, err := ctx.ExecuteTemplate(r.Client)
		if err != nil {
			return nil, errors.WrapPath(err, "client", "failed to get client")
		}
		client, err := newCustomServiceClient(r, reflect.ValueOf(x))
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	return newProtoClient(ctx, r, opts)
}

func (r *Request) appendMetadata(ctx *context.Context) (*context.Context, error) {
	if r.Metadata == nil {
		return ctx, nil
	}
	x, err := ctx.ExecuteTemplate(r.Metadata)
	if err != nil {
		return ctx, errors.WrapPathf(err, "metadata", "failed to set metadata")
	}
	md, err := reflectutil.ConvertStringsMap(reflect.ValueOf(x))
	if err != nil {
		return nil, errors.WrapPathf(err, "metadata", "failed to set metadata")
	}
	pairs := []string{}
	for k, vs := range md {
		for _, v := range vs {
			pairs = append(pairs, k, v)
		}
	}
	return ctx.WithRequestContext(
		metadata.AppendToOutgoingContext(ctx.RequestContext(), pairs...),
	), nil
}

func (r *Request) dumpRequest(ctx *context.Context, reqMsg proto.Message) *context.Context {
	//nolint:exhaustruct
	dumpReq := &request{
		Method:  r.Method,
		Message: &ProtoMessageYAMLMarshaler{reqMsg},
	}
	reqMD, _ := metadata.FromOutgoingContext(ctx.RequestContext())
	if len(reqMD) > 0 {
		dumpReq.Metadata = yamlutil.NewMDMarshaler(reqMD)
	}
	ctx = ctx.WithRequest((*RequestExtractor)(dumpReq))
	if b, err := yaml.Marshal(dumpReq); err == nil {
		ctx.Reporter().Logf("request:\n%s", r.addIndent(string(b), indentNum))
	} else {
		ctx.Reporter().Logf("failed to dump request:\n%s", err)
	}
	return ctx
}
