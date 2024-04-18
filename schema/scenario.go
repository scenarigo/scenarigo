// Package schema provides the test scenario data schema for scenarigo.
package schema

import (
	"fmt"
	"regexp"

	"github.com/goccy/go-yaml/ast"

	"github.com/zoncoen/scenarigo/errors"
	"github.com/zoncoen/scenarigo/protocol"
)

const stepIDPattern = `^[a-zA-Z0-9\-_]+$`

var stepIDRegexp = regexp.MustCompile(stepIDPattern)

// Scenario represents a test scenario.
type Scenario struct {
	SchemaVersion string            `yaml:"schemaVersion,omitempty"`
	Title         string            `yaml:"title,omitempty"`
	Description   string            `yaml:"description,omitempty"`
	Plugins       map[string]string `yaml:"plugins,omitempty"`
	Vars          map[string]any    `yaml:"vars,omitempty"`
	Secrets       map[string]any    `yaml:"secrets,omitempty"`
	Steps         []*Step           `yaml:"steps,omitempty"`

	// The strict YAML decoder fails to decode if finds an unknown field.
	// Anchors is the field for enabling to define YAML anchors by avoiding the error.
	// This field doesn't need to hold some data because anchors expand by the decoder.
	Anchors anchors `yaml:"anchors,omitempty"`

	filepath string   // YAML filepath
	Node     ast.Node `yaml:"-"`
}

// Filepath returns YAML filepath of s.
func (s *Scenario) Filepath() string {
	return s.filepath
}

// Validate validates a scenario.
func (s *Scenario) Validate() error {
	ids := map[string]struct{}{}
	for i, stp := range s.Steps {
		if stp.ID != "" {
			if !stepIDRegexp.MatchString(stp.ID) {
				return errors.WithNode(
					errors.ErrorPath(fmt.Sprintf("steps[%d].id", i), "step id must contain only alphanumeric characters, -, or _"),
					s.Node,
				)
			}
			if _, ok := ids[stp.ID]; ok {
				return errors.WithNode(
					errors.ErrorPathf(fmt.Sprintf("steps[%d].id", i), "step id %q is duplicated", stp.ID),
					s.Node,
				)
			}
			ids[stp.ID] = struct{}{}
		}

		if stp.Include == "" && stp.Ref == nil {
			if stp.Protocol == "" {
				return errors.WithNode(
					errors.ErrorPath(fmt.Sprintf("steps[%d]", i), "no protocol"),
					s.Node,
				)
			} else if protocol.Get(stp.Protocol) == nil {
				return errors.WithNode(
					errors.ErrorPathf(fmt.Sprintf("steps[%d].protocol", i), "protocol %q not found", stp.Protocol),
					s.Node,
				)
			}
		}
	}
	return nil
}

// Step represents a step of scenario.
type Step struct {
	ID                      string                    `yaml:"id,omitempty" validate:"alphanum"`
	Title                   string                    `yaml:"title,omitempty"`
	Description             string                    `yaml:"description,omitempty"`
	If                      string                    `yaml:"if,omitempty"`
	ContinueOnError         bool                      `yaml:"continueOnError,omitempty"`
	Vars                    map[string]any            `yaml:"vars,omitempty"`
	Secrets                 map[string]any            `yaml:"secrets,omitempty"`
	Protocol                string                    `yaml:"protocol,omitempty"`
	Request                 protocol.Invoker          `yaml:"request,omitempty"`
	Expect                  protocol.AssertionBuilder `yaml:"expect,omitempty"`
	Include                 string                    `yaml:"include,omitempty"`
	Ref                     interface{}               `yaml:"ref,omitempty"`
	Bind                    Bind                      `yaml:"bind,omitempty"`
	Timeout                 *Duration                 `yaml:"timeout,omitempty"`
	PostTimeoutWaitingLimit *Duration                 `yaml:"postTimeoutWaitingLimit,omitempty"`
	Retry                   *RetryPolicy              `yaml:"retry,omitempty"`
}

type rawMessage []byte

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (r *rawMessage) UnmarshalYAML(b []byte) error {
	*r = b
	return nil
}

type stepUnmarshaller struct {
	ID                      string         `yaml:"id,omitempty"`
	Title                   string         `yaml:"title,omitempty"`
	Description             string         `yaml:"description,omitempty"`
	If                      string         `yaml:"if,omitempty"`
	ContinueOnError         bool           `yaml:"continueOnError,omitempty"`
	Vars                    map[string]any `yaml:"vars,omitempty"`
	Secrets                 map[string]any `yaml:"secrets,omitempty"`
	Protocol                string         `yaml:"protocol,omitempty"`
	Include                 string         `yaml:"include,omitempty"`
	Ref                     interface{}    `yaml:"ref,omitempty"`
	Bind                    Bind           `yaml:"bind,omitempty"`
	Timeout                 *Duration      `yaml:"timeout,omitempty"`
	PostTimeoutWaitingLimit *Duration      `yaml:"postTimeoutWaitingLimit,omitempty"`
	Retry                   *RetryPolicy   `yaml:"retry,omitempty"`

	Request rawMessage `yaml:"request,omitempty"`
	Expect  rawMessage `yaml:"expect,omitempty"`
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (s *Step) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// unmarshal into stepUnmarshaller instead of Step for dynamic unmarshalling Request/Expect
	var unmarshaled stepUnmarshaller
	if err := unmarshal(&unmarshaled); err != nil {
		return err
	}

	s.ID = unmarshaled.ID
	s.Title = unmarshaled.Title
	s.Description = unmarshaled.Description
	s.If = unmarshaled.If
	s.ContinueOnError = unmarshaled.ContinueOnError
	s.Vars = unmarshaled.Vars
	s.Secrets = unmarshaled.Secrets
	s.Protocol = unmarshaled.Protocol
	s.Include = unmarshaled.Include
	s.Ref = unmarshaled.Ref
	s.Bind = unmarshaled.Bind
	s.Timeout = unmarshaled.Timeout
	s.PostTimeoutWaitingLimit = unmarshaled.PostTimeoutWaitingLimit
	s.Retry = unmarshaled.Retry

	p := protocol.Get(s.Protocol)
	if p == nil {
		if unmarshaled.Request != nil || unmarshaled.Expect != nil {
			return errors.Errorf("unknown protocol: %s", s.Protocol)
		}
		return nil
	}
	if unmarshaled.Request != nil {
		invoker, err := p.UnmarshalRequest(unmarshaled.Request)
		if err != nil {
			return err
		}
		s.Request = invoker
	}
	builder, err := p.UnmarshalExpect(unmarshaled.Expect)
	if err != nil {
		return err
	}
	s.Expect = builder

	return nil
}

// Bind represents bindings of variables.
type Bind struct {
	Vars    map[string]any `yaml:"vars,omitempty"`
	Secrets map[string]any `yaml:"secrets,omitempty"`
}

type anchors struct{}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (a anchors) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return nil
}
