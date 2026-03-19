package grpc

import (
	"bytes"
	"errors"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/zoncoen/query-go"
	protobufextractor "github.com/zoncoen/query-go/extractor/protobuf"

	"github.com/scenarigo/scenarigo/protocol"
)

var grpcProtocol = &GRPC{}

// Register registers grpc protocol.
func Register() {
	protocol.Register(grpcProtocol)
}

// GRPC is a protocol type for the scenarigo step.
type GRPC struct {
	m      sync.Mutex
	option Option
}

// Option represents a Option for gRPC.
type Option struct {
	Request *RequestOptions `yaml:"request,omitempty"`
}

// Name implements protocol.Protocol interface.
func (p *GRPC) Name() string {
	return "grpc"
}

// UnmarshalOption implements protocol.Protocol interface.
func (p *GRPC) UnmarshalOption(b []byte) error {
	p.m.Lock()
	defer p.m.Unlock()
	return yaml.UnmarshalWithOptions(b, &p.option, yaml.Strict())
}

func (p *GRPC) getOption() *Option {
	p.m.Lock()
	defer p.m.Unlock()
	return &p.option
}

// UnmarshalRequest implements protocol.Protocol interface.
func (p *GRPC) UnmarshalRequest(b []byte) (protocol.Invoker, error) {
	var r Request
	if err := yaml.UnmarshalWithOptions(b, &r, yaml.Strict()); err != nil {
		return nil, err
	}

	// for backward compatibility
	if r.Body != nil {
		if r.Message != nil {
			return nil, errors.New("body is deprecated, use message field only")
		}
		r.Message = r.Body
		r.Body = nil
	}

	return &r, nil
}

// UnmarshalExpect implements protocol.Protocol interface.
func (p *GRPC) UnmarshalExpect(b []byte) (protocol.AssertionBuilder, error) {
	var e Expect
	if b == nil {
		return &e, nil
	}
	decoder := yaml.NewDecoder(bytes.NewBuffer(b), yaml.UseOrderedMap(), yaml.Strict())
	if err := decoder.Decode(&e); err != nil {
		return nil, err
	}

	// for backward compatibility
	if e.Body != nil {
		if e.Message != nil {
			return nil, errors.New("body is deprecated, use message field only")
		}
		e.Message = e.Body
		e.Body = nil
	}

	return &e, nil
}

// QueryOptions implements the QueryOptionsProvider interface.
func (p *GRPC) QueryOptions() []query.Option {
	return []query.Option{
		query.CustomExtractFunc(protobufextractor.ExtractFunc()),
		query.CustomIsInlineStructFieldFunc(protobufextractor.OneofIsInlineStructFieldFunc()),
	}
}
