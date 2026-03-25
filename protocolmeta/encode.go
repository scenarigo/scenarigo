package protocolmeta

import "github.com/scenarigo/scenarigo/internal/protocolmeta"

// EncodeHTTPValue encodes a value for use in HTTP headers using URL path escaping.
func EncodeHTTPValue(value string) string {
	return protocolmeta.EncodeHTTPValue(value)
}
