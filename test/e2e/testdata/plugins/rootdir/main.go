package main

import (
	"os"
	"path/filepath"

	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/schema"
)

func init() {
	plugin.RegisterSetup(setup)
	plugin.RegisterSetupEachScenario(setupEachScenario)
}

func setup(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	check(ctx, "setup")
	return ctx, nil
}

func setupEachScenario(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	check(ctx, "setup each scenario")
	return ctx, nil
}

var CheckStep = plugin.StepFunc(func(ctx *plugin.Context, step *schema.Step) *plugin.Context {
	check(ctx, "step")
	return ctx
})

func Check(ctx *plugin.Context) bool {
	check(ctx, "function")
	return true
}

// check verifies that the scenario file can be found from the root directory.
func check(ctx *plugin.Context, name string) {
	dir := ctx.RootDir()
	if !filepath.IsAbs(dir) {
		ctx.Reporter().Fatalf("%s: root directory is not an absolute path: %q", name, dir)
	}
	path := filepath.Join(dir, "testdata", "testcases", "scenarios", "root-dir.yaml")
	if _, err := os.Stat(path); err != nil {
		ctx.Reporter().Fatalf("%s: %s", name, err)
	}
}
