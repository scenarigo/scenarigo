package context

import (
	"context"
	"os"

	query "github.com/zoncoen/query-go/v2"
)

var env = &envExtractor{}

type envExtractor struct{}

var _ query.KeyExtractor = (*envExtractor)(nil)

// ExtractByKey implements query.KeyExtractor interface.
func (f *envExtractor) ExtractByKey(_ context.Context, key string) (any, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil, query.ErrNotFound
	}
	return v, nil
}
