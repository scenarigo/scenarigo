package main

import (
	"fmt"
	"sync/atomic"

	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/schema"
)

var (
	attemptCounter uint64
)

// Nop is a step function that does nothing but allows us to check ScenarioFilepath in vars
var Nop = plugin.StepFunc(func(ctx *plugin.Context, step *schema.Step) *plugin.Context {
	return ctx
})

// CheckFilepath logs the current ScenarioFilepath from context
func CheckFilepath(ctx *plugin.Context) string {
	attempt := atomic.AddUint64(&attemptCounter, 1)
	filepath := ctx.ScenarioFilepath()

	fmt.Printf("CheckFilepath called - attempt %d\n", attempt)
	fmt.Printf("  ScenarioFilepath: '%s'\n", filepath)

	if filepath == "" {
		fmt.Printf("  WARNING: ScenarioFilepath is EMPTY!\n")
	}

	return filepath
}

// FailStep is a step function that always fails to trigger retry
var FailStep = plugin.StepFunc(func(ctx *plugin.Context, step *schema.Step) *plugin.Context {
	ctx.Reporter().Fatal("Intentional failure to trigger retry")
	return ctx
})

