package main

import (
	"sync/atomic"

	"github.com/scenarigo/scenarigo/plugin"
)

func init() {
	plugin.RegisterSetup(setupCounter)
}

func setupCounter(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	return ctx.WithVars(map[string]any{"counter": &Counter{}}), nil
}

type Counter struct {
	count uint64
}

func (c *Counter) Increment(ctx *plugin.Context) uint64 {
	i := atomic.AddUint64(&c.count, 1)
	ctx.Reporter().Printf("count: %d", i)
	return i
}
