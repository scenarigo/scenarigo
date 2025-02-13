package scenarigo

import (
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/plugin"
)

type setupFuncList []setupFunc

type setupFunc struct {
	name string
	f    plugin.SetupFunc
}

type teardownFunc struct {
	name string
	f    func(*plugin.Context)
}

func (sl setupFuncList) setup(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	if len(sl) == 0 {
		return ctx, func(_ *plugin.Context) {}
	}
	var teardowns []teardownFunc
	setupCtx := ctx
	ctx.Run("setup", func(ctx *plugin.Context) {
		for _, setup := range sl {
			if ctx.Reporter().Failed() {
				break
			}
			newCtx := ctx
			ctx.Run(setup.name, func(ctx *context.Context) {
				ctx, teardown := setup.f(ctx)
				if ctx != nil {
					newCtx = ctx
				}
				if teardown != nil {
					teardowns = append(teardowns, teardownFunc{
						name: setup.name,
						f:    teardown,
					})
				}
			})
			ctx = newCtx.WithReporter(ctx.Reporter())
		}
		setupCtx = ctx
	})
	ctx = setupCtx.WithReporter(ctx.Reporter())
	if len(teardowns) == 0 {
		return ctx, func(_ *plugin.Context) {}
	}
	return ctx, func(ctx *plugin.Context) {
		ctx.Run("teardown", func(ctx *plugin.Context) {
			for i := len(teardowns) - 1; i >= 0; i-- {
				ctx.Run(teardowns[i].name, func(ctx *context.Context) {
					teardowns[i].f(ctx)
				})
			}
		})
	}
}
