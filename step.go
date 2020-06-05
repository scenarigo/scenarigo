package scenarigo

import (
	"github.com/lestrrat-go/backoff"
	"golang.org/x/xerrors"

	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/plugin"
	"github.com/zoncoen/scenarigo/schema"
	"github.com/zoncoen/scenarigo/template"
)

func runStep(ctx *context.Context, s *schema.Step) *context.Context {
	if s.Vars != nil {
		vars, err := template.Execute(ctx, s.Vars, ctx)
		if err != nil {
			ctx.Reporter().Fatalf("invalid vars: %s", err)
		}
		ctx = ctx.WithVars(vars)
	}

	if s.Include != "" {
		scenarios, err := schema.LoadScenarios(ctx, s.Include)
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
		x, err := template.Execute(ctx, s.Ref, ctx)
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

	return invokeAndAssert(ctx, s)
}

func invokeAndAssert(ctx *context.Context, s *schema.Step) *context.Context {
	policy, err := s.Retry.Build()
	if err != nil {
		ctx.Reporter().Fatal(xerrors.Errorf("invalid retry policy: %w", err))
	}

	b, cancel := policy.Start(ctx.RequestContext())
	defer cancel()

	var i int
	for backoff.Continue(b) {
		ctx.Reporter().Logf("[%d] send request", i)
		i++
		newCtx, resp, err := s.Request.Invoke(ctx)
		if err != nil {
			ctx.Reporter().Log(err)
			continue
		}
		assertion, err := s.Expect.Build(newCtx)
		if err != nil {
			ctx.Reporter().Log(err)
			continue
		}
		if err := assertion.Assert(resp); err != nil {
			if assertErr, ok := err.(*assert.Error); ok {
				for _, err := range assertErr.Errors {
					ctx.Reporter().Log(err)
				}
			} else {
				ctx.Reporter().Log(err)
			}
			continue
		}
		return newCtx
	}

	ctx.Reporter().FailNow()
	return ctx
}
