package main

import (
	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/schema"
)

func init() {
	plugin.RegisterSetup(setup1)
	plugin.RegisterSetup(setup2)
	plugin.RegisterSetupEachScenario(setupEachScenario)
}

func setup1(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	ctx.Reporter().Log("setup 1")
	return ctx, nil
}

func setup2(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	ctx.Reporter().Log("setup 2")
	return ctx, func(ctx *plugin.Context) {
		ctx.Reporter().Log("teardown 2")
	}
}

func setupEachScenario(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	ctx.Reporter().Log("setup each scenario")
	return ctx, func(ctx *plugin.Context) {
		ctx.Reporter().Log("teardown each scenario")
	}
}

var NopStep = plugin.StepFunc(func(ctx *plugin.Context, step *schema.Step) *plugin.Context {
	ctx.Reporter().Log("nop step")
	return ctx
})

var FailStep = plugin.StepFunc(func(ctx *plugin.Context, step *schema.Step) *plugin.Context {
	ctx.Reporter().Fatal("fail step")
	return ctx
})
