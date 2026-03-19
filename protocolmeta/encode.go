package protocolmeta

import (
	"net/url"
)

// EncodeHTTPValue encodes a value for use in HTTP headers using URL path escaping.
func EncodeHTTPValue(value string) string {
	return url.PathEscape(value)
}
