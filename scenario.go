package scenarigo

import (
	gocontext "context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/errors"
	"github.com/zoncoen/scenarigo/plugin"
	"github.com/zoncoen/scenarigo/reporter"
	"github.com/zoncoen/scenarigo/schema"
)

// RunScenario runs a test scenario s.
func RunScenario(ctx *context.Context, s *schema.Scenario) *context.Context {
	ctx = ctx.WithScenarioFilepath(s.Filepath())
	steps := context.NewSteps()
	ctx = ctx.WithSteps(steps)

	var setups setupFuncList
	if s.Plugins != nil {
		plugs := map[string]interface{}{}
		for name, path := range s.Plugins {
			path := path
			if root := ctx.PluginDir(); root != "" {
				path = filepath.Join(root, path)
			}
			p, err := plugin.Open(path)
			if err != nil {
				ctx.Reporter().Fatalf(
					"failed to open plugin: %s",
					errors.WithNodeAndColored(
						errors.WithPath(err, fmt.Sprintf("plugins.'%s'", name)),
						ctx.Node(),
						ctx.EnabledColor(),
					),
				)
			}
			plugs[name] = p
			if setup := p.GetSetupEachScenario(); setup != nil {
				setups = append(setups, setupFunc{
					name: name,
					f:    setup,
				})
			}
		}
		ctx = ctx.WithPlugins(plugs)
	}

	if s.Vars != nil {
		vars, err := ctx.ExecuteTemplate(s.Vars)
		if err != nil {
			ctx.Reporter().Fatalf(
				"invalid vars: %s",
				errors.WithNodeAndColored(
					errors.WithPath(err, "vars"),
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
					errors.WithPath(err, "secrets"),
					ctx.Node(),
					ctx.EnabledColor(),
				),
			)
		}
		ctx = ctx.WithSecrets(secrets)
	}

	ctx, teardown := setups.setup(ctx)
	if ctx.Reporter().Failed() {
		if teardown != nil {
			teardown(ctx)
		}
		return ctx
	}

	scnCtx := ctx
	var failed bool
	for idx, step := range s.Steps {
		step := step
		var stepCtx *context.Context
		ok := context.RunWithRetry(scnCtx, step.Title, func(ctx *context.Context) {
			stepCtx = ctx

			// following steps are skipped if the previous step failed
			if failed {
				stepCtx.Reporter().SkipNow()
			}
			if run, err := executeIf(ctx, step.If); err != nil {
				stepCtx.Reporter().Fatal(
					errors.WithNodeAndColored(
						errors.WithPath(
							err,
							fmt.Sprintf("steps[%d].if", idx),
						),
						stepCtx.Node(),
						stepCtx.EnabledColor(),
					),
				)
			} else if !run {
				stepCtx.Reporter().SkipNow()
			}

			if step.ContinueOnError {
				reporter.NoFailurePropagation(stepCtx.Reporter())
			}

			if step.Timeout != nil && *step.Timeout > 0 {
				reqCtx, cancel := gocontext.WithTimeout(stepCtx.RequestContext(), time.Duration(*step.Timeout))
				defer cancel()
				stepCtx = stepCtx.WithRequestContext(reqCtx)
			}

			stepCtx = runStepWithTimeout(stepCtx, s, step, idx)

			// bind values to the scenario context for enable to access from following steps
			if step.Bind.Vars != nil {
				vars, err := stepCtx.ExecuteTemplate(step.Bind.Vars)
				if err != nil {
					stepCtx.Reporter().Fatal(
						errors.WithNodeAndColored(
							errors.WrapPath(
								err,
								fmt.Sprintf("steps[%d].bind.vars", idx),
								"invalid bind",
							),
							stepCtx.Node(),
							stepCtx.EnabledColor(),
						),
					)
				}
				scnCtx = scnCtx.WithVars(vars)
			}
			if step.Bind.Secrets != nil {
				secrets, err := stepCtx.ExecuteTemplate(step.Bind.Secrets)
				if err != nil {
					stepCtx.Reporter().Fatal(
						errors.WithNodeAndColored(
							errors.WrapPath(
								err,
								fmt.Sprintf("steps[%d].bind.secrets", idx),
								"invalid bind",
							),
							stepCtx.Node(),
							stepCtx.EnabledColor(),
						),
					)
				}
				scnCtx = scnCtx.WithSecrets(secrets)
				reporter.SetLogReplacer(stepCtx.Reporter(), scnCtx.Secrets())
			}
		}, step.Retry)
		if !ok && !step.ContinueOnError {
			failed = true
		}
		if stepCtx == nil {
			continue
		}
		if step.ID != "" {
			steps.Add(step.ID, &context.Step{ //nolint:exhaustruct
				Result: reporter.TestResultString(stepCtx.Reporter()),
			})
		}
	}

	if teardown != nil {
		teardown(scnCtx)
	}

	return scnCtx
}

func executeIf(ctx *context.Context, expr string) (bool, error) {
	if expr == "" {
		return true, nil
	}
	v, err := ctx.ExecuteTemplate(expr)
	if err != nil {
		return false, fmt.Errorf("failed to execute: %w", err)
	}
	run, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("must be bool but got %T", v)
	}
	return run, nil
}

func runStepWithTimeout(ctx *context.Context, scenario *schema.Scenario, step *schema.Step, idx int) *context.Context {
	done := make(chan *context.Context)
	go func() {
		var finished bool
		defer func() {
			if !finished {
				done <- ctx
			}
		}()
		done <- runStep(ctx, scenario, step, idx)
		finished = true
	}()
	select {
	case ctx = <-done:
	case <-ctx.RequestContext().Done():
		ctx.Reporter().Error(
			errors.WithNodeAndColored(
				errors.ErrorPath(
					fmt.Sprintf("steps[%d].timeout", idx),
					"timeout exceeded",
				),
				ctx.Node(),
				ctx.EnabledColor(),
			),
		)
		// wait for the result context for a little
		limit := 10 * time.Second
		if step.PostTimeoutWaitingLimit != nil {
			limit = time.Duration(*step.PostTimeoutWaitingLimit)
		}
		select {
		case ctx = <-done:
		case <-time.After(limit):
			go func() { <-done }()
			ctx.Reporter().Fatalf("step hasn't finished in %s despite the context canceled", limit)
		}
	}
	if ctx.Reporter().Failed() {
		ctx.Reporter().FailNow()
	}
	return ctx
}
