package context

import "os"

var env = &envExtractor{}

type envExtractor struct{}

// ExtractByKey implements query.KeyExtractor interface.
func (f *envExtractor) ExtractByKey(key string) (any, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil, false
	}
	return v, true
}
