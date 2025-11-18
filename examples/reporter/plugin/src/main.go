package main

import (
	"sync/atomic"

	"github.com/scenarigo/scenarigo/plugin"
)

var count uint64

// ctx (the first parameter) is automatically injected at call time; callers do not need to pass it explicitly.
func Increment(ctx *plugin.Context) uint64 {
	c := atomic.AddUint64(&count, 1)

	// This log is always emitted regardless of test results or log verbosity.
	ctx.Reporter().Printf("count: %d", c)

	// This log is emitted only when a test scenario fails or when the verbose log option is enabled.
	ctx.Reporter().Log("hidden log")

	return c
}
