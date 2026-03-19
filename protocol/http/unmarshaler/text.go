package unmarshaler

import (
	"errors"
	"reflect"
	"unicode/utf8"
)

func init() {
	if err := Register(&textUnmarshaler{}); err != nil {
		panic(err)
	}
}

type textUnmarshaler struct{}

// MediaType implements ResponseUnmarshaler interface.
func (um *textUnmarshaler) MediaType() string {
	return "text/plain"
}

// Unmarshal implements ResponseUnmarshaler interface.
func (um *textUnmarshaler) Unmarshal(data []byte, v any) error {
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
	if utf8.Valid(data) {
		rv.Set(reflect.ValueOf(string(data)))
	} else {
		rv.Set(reflect.ValueOf(data))
	}
	return nil
}
