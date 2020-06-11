package scenarigo

import (
	"fmt"

	"github.com/lestrrat-go/backoff"
	"golang.org/x/xerrors"

	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/errors"
	"github.com/zoncoen/scenarigo/plugin"
	"github.com/zoncoen/scenarigo/schema"
)

func runStep(ctx *context.Context, s *schema.Step, stepIdx int) *context.Context {
	if s.Vars != nil {
		vars, err := ctx.ExecuteTemplate(s.Vars)
		if err != nil {
			ctx.Reporter().Fatal(
				errors.WithNodeAndColored(
					errors.WrapPath(
						err,
						fmt.Sprintf("steps[%d].vars", stepIdx),
						"invalid vars",
					),
					ctx.Node(),
					ctx.EnabledColor(),
				),
			)
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
		includeNode, err := newYAMLNode(s.Include, 0)
		if err != nil {
			ctx.Reporter().Fatalf(`failed to create ast: %s`, err)
		}
		currentNode := ctx.Node()
		ctx = runScenario(ctx.WithNode(includeNode), scenarios[0])

		// back node to current node
		ctx = ctx.WithNode(currentNode)
		return ctx
	}
	if s.Ref != "" {
		x, err := ctx.ExecuteTemplate(s.Ref)
		if err != nil {
			ctx.Reporter().Fatal(
				errors.WithNodeAndColored(
					errors.WrapPathf(
						err,
						fmt.Sprintf("steps[%d].ref", stepIdx),
						`failed to reference "%s" as step`, s.Ref,
					),
					ctx.Node(),
					ctx.EnabledColor(),
				),
			)
		}
		stp, ok := x.(plugin.Step)
		if !ok {
			ctx.Reporter().Fatal(
				errors.WithNodeAndColored(
					errors.ErrorPathf(
						fmt.Sprintf("steps[%d].ref", stepIdx),
						`failed to reference "%s" as step: not implement plugin.Step interface`, s.Ref,
					),
					ctx.Node(),
					ctx.EnabledColor(),
				),
			)
		}
		ctx = stp.Run(ctx, s)
		return ctx
	}

	return invokeAndAssert(ctx, s, stepIdx)
}

func invokeAndAssert(ctx *context.Context, s *schema.Step, stepIdx int) *context.Context {
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
			ctx.Reporter().Log(
				errors.WithNodeAndColored(
					errors.WithPath(err, fmt.Sprintf("steps[%d].request", stepIdx)),
					ctx.Node(),
					ctx.EnabledColor(),
				),
			)
			continue
		}
		assertion, err := s.Expect.Build(newCtx)
		if err != nil {
			ctx.Reporter().Log(
				errors.WithNodeAndColored(
					errors.WithPath(err, fmt.Sprintf("steps[%d].expect", stepIdx)),
					ctx.Node(),
					ctx.EnabledColor(),
				),
			)
			continue
		}
		if err := assertion.Assert(resp); err != nil {
			err = errors.WithNodeAndColored(
				errors.WithPath(err, fmt.Sprintf("steps[%d].expect", stepIdx)),
				ctx.Node(),
				ctx.EnabledColor(),
			)
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
