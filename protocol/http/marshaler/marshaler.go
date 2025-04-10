package marshaler

import (
	"mime"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

// Default is the default request marshaler.
var Default = &jsonMarshaler{}

var (
	reqm        sync.Mutex
	reqRegistry = map[string]RequestMarshaler{}
)

// Register registers the request marshaler to the registry.
func Register(m RequestMarshaler) error {
	reqm.Lock()
	defer reqm.Unlock()
	mediaType, _, err := mime.ParseMediaType(strings.Trim(m.MediaType(), " "))
	if err != nil {
		return errors.Wrap(err, "failed to register request marshaler")
	}
	reqRegistry[mediaType] = m
	return nil
}

// Get returns the request marshaler for the given media type.
//
// If the marshaler is not found, returns the Default.
func Get(mediaType string) RequestMarshaler {
	reqm.Lock()
	defer reqm.Unlock()
	mt, _, err := mime.ParseMediaType(strings.Trim(mediaType, " "))
	if err != nil {
		return Default
	}
	m, ok := reqRegistry[mt]
	if !ok {
		return Default
	}
	return m
}

// RequestMarshaler is the interface that marshals the HTTP request body.
type RequestMarshaler interface {
	MediaType() string
	Marshal(v any) ([]byte, error)
}
