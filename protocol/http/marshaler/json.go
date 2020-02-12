package marshaler

import (
	"github.com/zoncoen/yaml"

	yamljson "sigs.k8s.io/yaml"
)

func init() {
	Register(&jsonMarshaler{})
}

type jsonMarshaler struct{}

// MediaType implements RequestMarshaler interface.
func (m *jsonMarshaler) MediaType() string {
	return "application/json"
}

// Marshal implements RequestMarshaler interface.
func (m *jsonMarshaler) Marshal(v interface{}) ([]byte, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	jb, err := yamljson.YAMLToJSON(b)
	if err != nil {
		return nil, err
	}
	return jb, nil
}
