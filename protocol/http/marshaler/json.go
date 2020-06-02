package marshaler

import (
	"encoding/json"
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
func (m *jsonMarshaler) Marshal(v interface{}) ([]byte, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
