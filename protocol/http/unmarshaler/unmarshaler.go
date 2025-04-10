package unmarshaler

import (
	"mime"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

// Default is the default response unmarshaler.
var Default = &binaryUnmarshaler{}

var (
	resm        sync.Mutex
	resRegistry = map[string]ResponseUnmarshaler{}
)

// Register registers the response unmarshaler to the registry.
func Register(um ResponseUnmarshaler) error {
	resm.Lock()
	defer resm.Unlock()
	mediaType, _, err := mime.ParseMediaType(strings.Trim(um.MediaType(), " "))
	if err != nil {
		return errors.Wrap(err, "failed to register response unmarshaler")
	}
	resRegistry[mediaType] = um
	return nil
}

// Get returns the response unmarshaler for the given media type.
//
// If the unmarshaler is not found, returns the Default.
func Get(mediaType string) ResponseUnmarshaler {
	resm.Lock()
	defer resm.Unlock()
	mt, _, err := mime.ParseMediaType(strings.Trim(mediaType, " "))
	if err != nil {
		return Default
	}
	um, ok := resRegistry[mt]
	if !ok {
		return Default
	}
	return um
}

// ResponseUnmarshaler is the interface that unmarshals the HTTP response body.
type ResponseUnmarshaler interface {
	MediaType() string
	Unmarshal(data []byte, v any) error
}
