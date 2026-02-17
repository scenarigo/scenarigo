package main

import (
	"fmt"

	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/schema"
)

func init() {
	plugin.RegisterSetupEachScenario(setup)
}

func setup(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	return ctx, func(ctx *plugin.Context) {
		reqCtx := ctx.RequestContext()
		if err := reqCtx.Err(); err != nil {
			fmt.Printf("BUG: teardown received already-cancelled RequestContext: %s\n", err)
		} else {
			fmt.Printf("OK: teardown received valid RequestContext\n")
		}
	}
}

// Nop is a step function that does nothing.
var Nop = plugin.StepFunc(func(ctx *plugin.Context, step *schema.Step) *plugin.Context {
	return ctx
})
