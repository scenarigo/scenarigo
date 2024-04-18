package queryutil

import (
	"sync"

	query "github.com/zoncoen/query-go"
	yamlextractor "github.com/zoncoen/query-go/extractor/yaml"
)

var (
	m    sync.RWMutex
	opts = []query.Option{}
)

func New(opts ...query.Option) *query.Query {
	return query.New(append(Options(), opts...)...)
}

func Options() []query.Option {
	m.RLock()
	defer m.RUnlock()
	return append(
		[]query.Option{
			query.ExtractByStructTag("yaml", "json"),
			query.CustomExtractFunc(yamlextractor.MapSliceExtractFunc()),
		},
		opts...,
	)
}

func AppendOptions(customOpts ...query.Option) {
	m.Lock()
	defer m.Unlock()
	opts = append(opts, customOpts...)
}
