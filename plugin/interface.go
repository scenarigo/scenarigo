package plugin

import (
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/schema"
	"github.com/zoncoen/scenarigo/template"
)

// Context represents a scenarigo context.
type Context = context.Context

// LeftArrowFunc represents a left arrow function.
type LeftArrowFunc = template.Func

// Step represents a step plugin.
type Step interface {
	Run(*context.Context, *schema.Step) *context.Context
}

// StepFunc is an adaptor to allow the use of ordinary functions as step.
type StepFunc func(ctx *context.Context, step *schema.Step) *context.Context

// Run implements Step interface.
func (f StepFunc) Run(ctx *context.Context, step *schema.Step) *context.Context {
	return f(ctx, step)
}
