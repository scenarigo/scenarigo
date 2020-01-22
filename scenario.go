package scenarigo

import (
	"path/filepath"
	"plugin"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/schema"
)

func runScenario(ctx *context.Context, s *schema.Scenario) *context.Context {
	if s.Plugins != nil {
		plugs := map[string]*plugin.Plugin{}
		for name, path := range s.Plugins {
			path := path
			if root := ctx.PluginDir(); root != "" {
				path = filepath.Join(root, path)
			}
			plug, err := plugin.Open(path)
			if err != nil {
				ctx.Reporter().Fatalf("failed to open plugin: %s", err)
			}
			plugs[name] = plug
		}
		ctx = ctx.WithPlugins(plugs)
	}

	if s.Vars != nil {
		vars, err := ctx.ExecuteTemplate(s.Vars)
		if err != nil {
			ctx.Reporter().Fatalf("invalid vars: %s", err)
		}
		ctx = ctx.WithVars(vars)
	}

	scnCtx := ctx
	var failed bool
	for _, step := range s.Steps {
		step := step
		ok := scnCtx.Run(step.Title, func(ctx *context.Context) {
			// following steps are skipped if the previous step failed
			if failed {
				ctx.Reporter().SkipNow()
			}

			if step.Include != "" {
				step.Include = filepath.Join(filepath.Dir(s.Filepath()), step.Include)
			}
			ctx = runStep(ctx, step)

			// bind values to the scenario context for enable to access from following steps
			if step.Bind.Vars != nil {
				vars, err := ctx.ExecuteTemplate(step.Bind.Vars)
				if err != nil {
					ctx.Reporter().Fatalf("invalid bind: %s", err)
				}
				scnCtx = scnCtx.WithVars(vars)
			}
		})
		if !failed {
			failed = !ok
		}
	}

	return scnCtx
}
