package marshaler

import "errors"

func init() {
	if err := Register(&textMarshaler{}); err != nil {
		panic(err)
	}
}

type textMarshaler struct{}

// MediaType implements RequestMarshaler interface.
func (m *textMarshaler) MediaType() string {
	return "text/plain"
}

// Marshal implements RequestMarshaler interface.
func (m *textMarshaler) Marshal(v interface{}) ([]byte, error) {
	b, ok := v.([]byte)
	if !ok {
		return nil, errors.New("v must be []byte")
	}
	return b, nil
}
