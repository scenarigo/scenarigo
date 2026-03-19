package schema

import (
	"regexp"

	"github.com/goccy/go-yaml"
)

// Regexp represents a regular expression pattern.
type Regexp struct {
	*regexp.Regexp

	str string
}

// String returns a string representing r.
func (r Regexp) String() string {
	return r.str
}

// MarshalYAML implements yaml.BytesMarshaler interface.
func (r Regexp) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(r.str)
}

// UnmarshalYAML implements yaml.BytesUnmarshaler interface.
func (r *Regexp) UnmarshalYAML(b []byte) error {
	var s string
	if err := yaml.Unmarshal(b, &s); err != nil {
		return err
	}
	re, err := regexp.Compile(s)
	if err != nil {
		return err
	}
	r.str = s
	r.Regexp = re
	return nil
}
