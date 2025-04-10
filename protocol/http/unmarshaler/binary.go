package unmarshaler

import (
	"errors"
	"reflect"
)

func init() {
	if err := Register(&binaryUnmarshaler{}); err != nil {
		panic(err)
	}
}

type binaryUnmarshaler struct{}

// MediaType implements ResponseUnmarshaler interface.
// It returns a wildcard type to denote that binaryUnmarshaler is the fallback unmarshaler
// used for responses that do not mach a registered media type.
func (um *binaryUnmarshaler) MediaType() string {
	return "*/*"
}

// Unmarshal implements ResponseUnmarshaler interface.
// It unmarshals the data as just a sequence of bytes.
func (um *binaryUnmarshaler) Unmarshal(data []byte, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return errors.New("v must be a pointer")
	}
	if rv.IsNil() {
		return errors.New("v is nil")
	}
	rv = rv.Elem()
	if !rv.CanSet() {
		return errors.New("v is not settable")
	}
	rv.Set(reflect.ValueOf(data))
	return nil
}
