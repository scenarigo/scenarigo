package marshaler

import (
	"net/url"
	"reflect"

	"github.com/scenarigo/scenarigo/internal/reflectutil"
)

func init() {
	if err := Register(&formURLEncodedMarshaler{}); err != nil {
		panic(err)
	}
}

type formURLEncodedMarshaler struct{}

// MediaType implements RequestMarshaler interface.
func (m *formURLEncodedMarshaler) MediaType() string {
	return "application/x-www-form-urlencoded"
}

// Marshal implements RequestMarshaler interface.
func (m *formURLEncodedMarshaler) Marshal(v interface{}) ([]byte, error) {
	form, err := reflectutil.ConvertStringsMap(reflect.ValueOf(v))
	if err != nil {
		return nil, err
	}
	return []byte(url.Values(form).Encode()), nil
}
