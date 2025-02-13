package scenarigo

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
)

func runStep(ctx *context.Context, scenario *schema.Scenario, s *schema.Step, stepIdx int) *context.Context {
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
	if s.Secrets != nil {
		secrets, err := ctx.ExecuteTemplate(s.Secrets)
		if err != nil {
			ctx.Reporter().Fatalf(
				"invalid secrets: %s",
				errors.WithNodeAndColored(
					errors.WithPath(err, fmt.Sprintf("steps[%d].secrets", stepIdx)),
					ctx.Node(),
					ctx.EnabledColor(),
				),
			)
		}
		ctx = ctx.WithSecrets(secrets)
	}

	if s.Include != "" {
		baseDir := filepath.Dir(scenario.Filepath())
		include := filepath.Join(baseDir, s.Include)
		scenarios, err := schema.LoadScenarios(include)
		if err != nil {
			ctx.Reporter().Fatalf(`failed to include "%s" as step: %s`, s.Include, err)
		}
		if len(scenarios) != 1 {
			ctx.Reporter().Fatalf(`failed to include "%s" as step: must be a scenario`, s.Include)
		}
		testName, err := filepath.Rel(baseDir, include)
		if err != nil {
			ctx.Reporter().Fatalf(`failed to include "%s" as step: %s`, s.Include, err)
		}
		currentNode := ctx.Node()
		ctx.Reporter().Run(testName, func(rptr reporter.Reporter) {
			ctx = RunScenario(ctx.WithReporter(rptr).WithNode(scenarios[0].Node), scenarios[0])
		})
		if ctx.Reporter().Failed() {
			ctx.Reporter().FailNow()
		}

		// back node to current node
		ctx = ctx.WithNode(currentNode)
		return ctx
	}
	if s.Ref != nil {
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
		startTime := time.Now()
		ctx = stp.Run(ctx, s)
		ctx.Reporter().Logf("Run %s: elapsed time %f sec", s.Ref, time.Since(startTime).Seconds())
		return ctx
	}

	return invokeAndAssert(ctx, s, stepIdx)
}

func invokeAndAssert(ctx *context.Context, s *schema.Step, stepIdx int) *context.Context {
	reqTime := time.Now()
	newCtx, resp, err := s.Request.Invoke(ctx)
	ctx.Reporter().Logf("elapsed time: %f sec", time.Since(reqTime).Seconds())

	if err != nil {
		ctx.Reporter().Fatal(
			errors.WithNodeAndColored(
				errors.WithPath(err, fmt.Sprintf("steps[%d].request", stepIdx)),
				ctx.Node(),
				ctx.EnabledColor(),
			),
		)
	}
	assertion, err := s.Expect.Build(newCtx)
	if err != nil {
		ctx.Reporter().Fatal(
			errors.WithNodeAndColored(
				errors.WithPath(err, fmt.Sprintf("steps[%d].expect", stepIdx)),
				ctx.Node(),
				ctx.EnabledColor(),
			),
		)
	}
	if err := assertion.Assert(resp); err != nil {
		err = errors.WithNodeAndColored(
			errors.WithPath(err, fmt.Sprintf("steps[%d].expect", stepIdx)),
			ctx.Node(),
			ctx.EnabledColor(),
		)
		var assertErr *assert.Error
		if errors.As(err, &assertErr) {
			for _, err := range assertErr.Errors {
				ctx.Reporter().Error(err)
			}
		} else {
			ctx.Reporter().Error(err)
		}
		ctx.Reporter().FailNow()
	}
	return newCtx
}
