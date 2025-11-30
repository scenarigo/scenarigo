// Package schema provides the test scenario data schema for scenarigo.
package schema

import (
	"fmt"
	"regexp"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"

	"github.com/scenarigo/scenarigo/color"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/protocol"
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
	Retry         *RetryPolicy      `yaml:"retry,omitempty"`

	// The strict YAML decoder fails to decode if finds an unknown field.
	// Anchors is the field for enabling to define YAML anchors by avoiding the error.
	// This field doesn't need to hold some data because anchors expand by the decoder.
	Anchors anchors `yaml:"anchors,omitempty"`

	filepath    string        // YAML filepath
	Node        ast.Node      `yaml:"-"`
	colorConfig *color.Config // Color configuration for error reporting

	yamlBytes []byte
}

type scenarioUnmarshaler Scenario

func (s *Scenario) UnmarshalYAML(b []byte) error {
	var unm scenarioUnmarshaler
	if err := yaml.UnmarshalWithOptions(b, &unm, yaml.UseOrderedMap(), yaml.Strict()); err != nil {
		return err
	}
	*s = Scenario(unm)
	if s.Retry != nil {
		s.yamlBytes = b
	}
	return nil
}

func (s *Scenario) Reset() error {
	return yaml.UnmarshalWithOptions(s.yamlBytes, s, yaml.UseOrderedMap(), yaml.Strict())
}

// Filepath returns YAML filepath of s.
func (s *Scenario) Filepath() string {
	return s.filepath
}

// setColorConfig sets the color configuration for error reporting.
func (s *Scenario) setColorConfig(config *color.Config) {
	s.colorConfig = config
}

// getColorEnabled returns whether color is enabled for error reporting.
func (s *Scenario) getColorEnabled() bool {
	if s.colorConfig == nil {
		s.colorConfig = color.New()
	}
	return s.colorConfig.IsEnabled()
}

// Validate validates a scenario.
func (s *Scenario) Validate() error {
	ids := map[string]struct{}{}
	for i, stp := range s.Steps {
		if stp.ID != "" {
			if !stepIDRegexp.MatchString(stp.ID) {
				return errors.WithNodeAndColored(
					errors.ErrorPath(fmt.Sprintf("steps[%d].id", i), "step id must contain only alphanumeric characters, -, or _"),
					s.Node,
					s.getColorEnabled(),
				)
			}
			if _, ok := ids[stp.ID]; ok {
				return errors.WithNodeAndColored(
					errors.ErrorPathf(fmt.Sprintf("steps[%d].id", i), "step id %q is duplicated", stp.ID),
					s.Node,
					s.getColorEnabled(),
				)
			}
			ids[stp.ID] = struct{}{}
		}

		if stp.Include == "" && stp.Ref == nil {
			if stp.Protocol == "" {
				return errors.WithNodeAndColored(
					errors.ErrorPath(fmt.Sprintf("steps[%d]", i), "no protocol"),
					s.Node,
					s.getColorEnabled(),
				)
			} else if protocol.Get(stp.Protocol) == nil {
				return errors.WithNodeAndColored(
					errors.ErrorPathf(fmt.Sprintf("steps[%d].protocol", i), "protocol %q not found", stp.Protocol),
					s.Node,
					s.getColorEnabled(),
				)
			}
		}
	}
	return nil
}

// Step represents a step of scenario.
type Step struct {
	ID                      string                    `validate:"alphanum"                      yaml:"id,omitempty"`
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
	Ref                     any                       `yaml:"ref,omitempty"`
	Bind                    Bind                      `yaml:"bind,omitempty"`
	Timeout                 *Duration                 `yaml:"timeout,omitempty"`
	PostTimeoutWaitingLimit *Duration                 `yaml:"postTimeoutWaitingLimit,omitempty"`
	Retry                   *RetryPolicy              `yaml:"retry,omitempty"`

	yamlBytes []byte
}

// RawMessage is a raw encoded YAML value.
type RawMessage []byte

// MarshalYAML implements yaml.BytesMarshaler interface.
func (r RawMessage) MarshalYAML() ([]byte, error) {
	return []byte(r), nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (r *RawMessage) UnmarshalYAML(b []byte) error {
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
	Ref                     any            `yaml:"ref,omitempty"`
	Bind                    Bind           `yaml:"bind,omitempty"`
	Timeout                 *Duration      `yaml:"timeout,omitempty"`
	PostTimeoutWaitingLimit *Duration      `yaml:"postTimeoutWaitingLimit,omitempty"`
	Retry                   *RetryPolicy   `yaml:"retry,omitempty"`

	Request RawMessage `yaml:"request,omitempty"`
	Expect  RawMessage `yaml:"expect,omitempty"`
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (s *Step) UnmarshalYAML(b []byte) error {
	// unmarshal into stepUnmarshaller instead of Step for dynamic unmarshalling Request/Expect
	var unmarshaled stepUnmarshaller
	if err := yaml.UnmarshalWithOptions(b, &unmarshaled, yaml.UseOrderedMap(), yaml.Strict()); err != nil {
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

	if s.Retry != nil {
		s.yamlBytes = b
	}

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

func (s *Step) Reset() error {
	return yaml.UnmarshalWithOptions(s.yamlBytes, s, yaml.UseOrderedMap(), yaml.Strict())
}

// Bind represents bindings of variables.
type Bind struct {
	Vars    map[string]any `yaml:"vars,omitempty"`
	Secrets map[string]any `yaml:"secrets,omitempty"`
}

type anchors struct{}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (a anchors) UnmarshalYAML(unmarshal func(any) error) error {
	return nil
}
