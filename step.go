package scenarigo

import (
	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/plugin"
	"github.com/zoncoen/scenarigo/schema"
)

func runStep(ctx *context.Context, s *schema.Step) *context.Context {
	if s.Vars != nil {
		vars, err := ctx.ExecuteTemplate(s.Vars)
		if err != nil {
			ctx.Reporter().Fatalf("invalid vars: %s", err)
		}
		ctx = ctx.WithVars(vars)
	}

	if s.Include != "" {
		scenarios, err := schema.LoadScenarios(s.Include)
		if err != nil {
			ctx.Reporter().Fatalf(`failed to include "%s" as step: %s`, s.Include, err)
		}
		if len(scenarios) != 1 {
			ctx.Reporter().Fatalf(`failed to include "%s" as step: must be a scenario`, s.Include)
		}
		ctx = runScenario(ctx, scenarios[0])
		return ctx
	}
	if s.Ref != "" {
		x, err := ctx.ExecuteTemplate(s.Ref)
		if err != nil {
			ctx.Reporter().Fatalf(`failed to reference "%s" as step: %s`, s.Ref, err)
		}
		stp, ok := x.(plugin.Step)
		if !ok {
			ctx.Reporter().Fatalf(`failed to reference "%s" as step: not implement plugin.Step interface`, s.Ref)
		}
		ctx = stp.Run(ctx, s)
		return ctx
	}

	newCtx, resp, err := s.Request.Invoke(ctx)
	if err != nil {
		ctx.Reporter().Fatal(err)
	}
	ctx = newCtx

	assertion, err := s.Expect.Build(ctx)
	if err != nil {
		ctx.Reporter().Fatal(err)
	}
	if err := assertion.Assert(resp); err != nil {
		if assertErr, ok := err.(*assert.Error); ok {
			for _, err := range assertErr.Errors {
				ctx.Reporter().Error(err)
			}
			ctx.Reporter().FailNow()
		} else {
			ctx.Reporter().Fatal(err)
		}
	}

	return ctx
}
