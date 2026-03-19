package protocolmeta

import public "github.com/scenarigo/scenarigo/protocolmeta"

// EncodeHTTPValue encodes a value for use in HTTP headers using URL path escaping.
func EncodeHTTPValue(value string) string {
	return public.EncodeHTTPValue(value)
}
