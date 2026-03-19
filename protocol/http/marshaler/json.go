package marshaler

import (
	"bytes"

	"github.com/goccy/go-yaml"
)

func init() {
	if err := Register(&jsonMarshaler{}); err != nil {
		panic(err)
	}
}

type jsonMarshaler struct{}

// MediaType implements RequestMarshaler interface.
func (m *jsonMarshaler) MediaType() string {
	return "application/json"
}

// Marshal implements RequestMarshaler interface.
func (m *jsonMarshaler) Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := yaml.NewEncoder(&buf, yaml.JSON()).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
