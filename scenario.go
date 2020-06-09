package scenarigo

import (
	"fmt"
	"path/filepath"
	"plugin"
	"reflect"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/errors"
	"github.com/zoncoen/scenarigo/schema"
)

func runScenario(ctx *context.Context, s *schema.Scenario) *context.Context {
	if s.Plugins != nil {
		plugs := map[string]interface{}{}
		for name, path := range s.Plugins {
			path := path
			if root := ctx.PluginDir(); root != "" {
				path = filepath.Join(root, path)
			}
			p, err := plugin.Open(path)
			if err != nil {
				ctx.Reporter().Fatalf("failed to open plugin: %s", err)
			}
			plugs[name] = &plug{p}
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
	for idx, step := range s.Steps {
		step := step
		ok := scnCtx.Run(step.Title, func(ctx *context.Context) {
			// following steps are skipped if the previous step failed
			if failed {
				ctx.Reporter().SkipNow()
			}

			if step.Include != "" {
				step.Include = filepath.Join(filepath.Dir(s.Filepath()), step.Include)
			}
			ctx = runStep(ctx, step, idx)

			// bind values to the scenario context for enable to access from following steps
			if step.Bind.Vars != nil {
				vars, err := ctx.ExecuteTemplate(step.Bind.Vars)
				if err != nil {
					ctx.Reporter().Fatal(
						errors.WithNodeAndColored(
							errors.WrapPath(
								err,
								fmt.Sprintf("steps[%d].bind.vars", idx),
								"invalid bind",
							),
							ctx.Node(),
							ctx.EnabledColor(),
						),
					)
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

// lookupper is an interface wrapper around *plugin.Plugin.
// NOTE: If we use plugin.Plugin in tests, go test with -race flag will fail.
type lookupper interface {
	Lookup(string) (plugin.Symbol, error)
}

type plug struct {
	lookupper
}

// ExtractByKey implements query.KeyExtractor interface.
func (p *plug) ExtractByKey(key string) (interface{}, bool) {
	if sym, err := p.Lookup(key); err == nil {
		// If sym is a pointer to a variable, return the actual variable for convenience.
		if v := reflect.ValueOf(sym); v.Kind() == reflect.Ptr {
			return v.Elem().Interface(), true
		}
		return sym, true
	}
	return nil, false
}
