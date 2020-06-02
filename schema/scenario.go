// Package schema provides the test scenario data schema for scenarigo.
package schema

import (
	"github.com/pkg/errors"
	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/protocol"
)

// Scenario represents a test scenario.
type Scenario struct {
	Title       string                 `yaml:"title"`
	Description string                 `yaml:"description"`
	Plugins     map[string]string      `yaml:"plugins"`
	Vars        map[string]interface{} `yaml:"vars"`
	Steps       []*Step                `yaml:"steps"`

	// The strict YAML decoder fails to decode if finds an unknown field.
	// Anchors is the field for enabling to define YAML anchors by avoiding the error.
	// This field doesn't need to hold some data because anchors expand by the decoder.
	Anchors anchors `yaml:"anchors"`

	filepath string // YAML filepath
}

// Filepath returns YAML filepath of s.
func (s *Scenario) Filepath() string {
	return s.filepath
}

// Step represents a step of scenario.
type Step struct {
	Title       string                 `yaml:"title"`
	Description string                 `yaml:"description"`
	Vars        map[string]interface{} `yaml:"vars"`
	Protocol    string                 `yaml:"protocol"`
	Request     Request                `yaml:"request"`
	Expect      Expect                 `yaml:"expect"`
	Include     string                 `yaml:"include"`
	Ref         string                 `yaml:"ref"`
	Bind        Bind                   `yaml:"bind"`
	Retry       *RetryPolicy           `yaml:"retry"`
}

type stepUnmarshaller Step

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (s *Step) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*stepUnmarshaller)(s)); err != nil {
		return err
	}

	p := protocol.Get(s.Protocol)
	if p == nil {
		if s.Request.bytes != nil || s.Expect.bytes != nil {
			return errors.Errorf("unknown protocol: %s", s.Protocol)
		}
	}
	if s.Request.bytes != nil {
		invoker, err := p.UnmarshalRequest(s.Request.bytes)
		if err != nil {
			return err
		}
		s.Request.Invoker = invoker
	}
	if s.Expect.bytes != nil {
		builder, err := p.UnmarshalExpect(s.Expect.bytes)
		if err != nil {
			return err
		}
		s.Expect.AssertionBuilder = builder
	}

	return nil
}

// Request represents a request.
type Request struct {
	protocol.Invoker
	bytes []byte
}

// Invoke sends the request.
func (r *Request) Invoke(ctx *context.Context) (*context.Context, interface{}, error) {
	if r.Invoker == nil {
		return ctx, nil, errors.New("invalid request")
	}
	return r.Invoker.Invoke(ctx)
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (r *Request) UnmarshalYAML(bytes []byte) error {
	r.bytes = bytes
	return nil
}

// Expect represents a expect response.
type Expect struct {
	protocol.AssertionBuilder
	bytes []byte
}

// Build builds the assertion which asserts the response.
func (e *Expect) Build(ctx *context.Context) (assert.Assertion, error) {
	if e.AssertionBuilder == nil {
		return assert.AssertionFunc(func(v interface{}) error {
			return nil
		}), nil
	}
	return e.AssertionBuilder.Build(ctx)
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (e *Expect) UnmarshalYAML(bytes []byte) error {
	e.bytes = bytes
	return nil
}

// Bind represents bindings of variables.
type Bind struct {
	Vars map[string]interface{} `yaml:"vars"`
}

type anchors struct{}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (a anchors) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return nil
}
