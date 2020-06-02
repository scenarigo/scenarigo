package http

import (
	"github.com/goccy/go-yaml"
	"github.com/zoncoen/scenarigo/protocol"
)

func init() {
	protocol.Register(&HTTP{})
}

// HTTP is a protocol type for the scenarigo step.
type HTTP struct{}

// Name implements protocol.Protocol interface.
func (p *HTTP) Name() string {
	return "http"
}

// UnmarshalRequest implements protocol.Protocol interface.
func (p *HTTP) UnmarshalRequest(bytes []byte) (protocol.Invoker, error) {
	var r Request
	if err := yaml.Unmarshal(bytes, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// UnmarshalExpect implements protocol.Protocol interface.
func (p *HTTP) UnmarshalExpect(unmarshal func(interface{}) error) (protocol.AssertionBuilder, error) {
	var e Expect
	if err := unmarshal(&e); err != nil {
		return nil, err
	}
	return &e, nil
}
