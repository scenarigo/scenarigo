package main

import (
	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/schema"
)

func init() {
	plugin.RegisterSetupEachScenario(setupEachScenario)
}

func setupEachScenario(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	if err := ctx.RequestContext().Err(); err != nil {
		ctx.Reporter().Fatalf("setup received canceled RequestContext: %s", err)
	}
	ctx.Reporter().Log("setup: RequestContext is valid")
	return ctx, func(ctx *plugin.Context) {
		if err := ctx.RequestContext().Err(); err != nil {
			ctx.Reporter().Fatalf("teardown received canceled RequestContext: %s", err)
		}
		ctx.Reporter().Log("teardown: RequestContext is valid")
	}
}

var CheckReqCtxStep = plugin.StepFunc(func(ctx *plugin.Context, step *schema.Step) *plugin.Context {
	if err := ctx.RequestContext().Err(); err != nil {
		ctx.Reporter().Fatalf("step received canceled RequestContext: %s", err)
	}
	ctx.Reporter().Log("step: RequestContext is valid")
	return ctx
})
