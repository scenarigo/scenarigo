package schema

import (
	"time"

	"github.com/goccy/go-yaml"
)

// Duration represents the elapsed time.
type Duration time.Duration

// String returns a string representing d.
func (d Duration) String() string {
	return time.Duration(d).String()
}

// MarshalYAML implements yaml.BytesMarshaler interface.
func (d *Duration) MarshalYAML() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalYAML implements yaml.BytesUnmarshaler interface.
func (d *Duration) UnmarshalYAML(b []byte) error {
	var s string
	if err := yaml.Unmarshal(b, &s); err != nil {
		return err
	}
	in, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(in)
	return nil
}
