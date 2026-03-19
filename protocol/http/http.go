package http

import (
	"bytes"

	"github.com/goccy/go-yaml"
	"github.com/scenarigo/scenarigo/protocol"
)

// Register registers http protocol.
func Register() {
	protocol.Register(&HTTP{})
}

// HTTP is a protocol type for the scenarigo step.
type HTTP struct{}

// Name implements protocol.Protocol interface.
func (p *HTTP) Name() string {
	return "http"
}

// UnmarshalOption implements protocol.Protocol interface.
func (p *HTTP) UnmarshalOption(_ []byte) error {
	return nil
}

// UnmarshalRequest implements protocol.Protocol interface.
func (p *HTTP) UnmarshalRequest(b []byte) (protocol.Invoker, error) {
	var r Request
	if err := yaml.UnmarshalWithOptions(b, &r, yaml.Strict()); err != nil {
		return nil, err
	}
	return &r, nil
}

// UnmarshalExpect implements protocol.Protocol interface.
func (p *HTTP) UnmarshalExpect(b []byte) (protocol.AssertionBuilder, error) {
	var e Expect
	if b == nil {
		return &e, nil
	}
	decoder := yaml.NewDecoder(bytes.NewBuffer(b), yaml.UseOrderedMap(), yaml.Strict())
	if err := decoder.Decode(&e); err != nil {
		return nil, err
	}
	return &e, nil
}
