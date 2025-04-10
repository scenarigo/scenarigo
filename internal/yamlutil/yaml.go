package yamlutil

import (
	"errors"

	"github.com/goccy/go-yaml"
)

// RawMessage is a raw encoded YAML value.
type RawMessage []byte

// UnmarshalYAML decodes bytes into msg.
func (msg *RawMessage) UnmarshalYAML(bytes []byte) error {
	if msg == nil {
		return errors.New("unmarshal YAML into nil value")
	}
	*msg = bytes
	return nil
}

// Unmarshal decodes msg into v.
func (msg RawMessage) Unmarshal(v any) error {
	return yaml.UnmarshalWithOptions([]byte(msg), v, yaml.UseOrderedMap(), yaml.Strict())
}
