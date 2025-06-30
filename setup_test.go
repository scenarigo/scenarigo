package scenarigo

import (
	"bytes"
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/testutil"
	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/sergi/go-diff/diffmatchpatch"
)

func TestSetupMap_Setup(t *testing.T) {
	tests := map[string]struct {
		setups setupFuncList
		failed bool
		expect string
	}{
		"nil": {
			setups: nil,
			failed: false,
		},
		"empty": {
			setups: setupFuncList{},
			failed: false,
		},
		"no teardown": {
			setups: setupFuncList{
				{
					name: "a",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup a")
						return ctx, nil
					},
				},
				{
					name: "b",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup b")
						return ctx, nil
					},
				},
				{
					name: "c",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup c")
						return ctx, nil
					},
				},
			},
			failed: false,
			expect: `
=== RUN   setup
=== RUN   setup/a
=== RUN   setup/b
=== RUN   setup/c
--- PASS: setup (0.00s)
    --- PASS: setup/a (0.00s)
            setup a
    --- PASS: setup/b (0.00s)
            setup b
    --- PASS: setup/c (0.00s)
            setup c
PASS
ok  	setup	0.000s
`,
		},
		"with teardown": {
			setups: setupFuncList{
				{
					name: "a",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup a")
						ctx = ctx.WithVars(map[string]int{"a": 1})
						return ctx, func(ctx *plugin.Context) {
							v, ok := ctx.Vars().ExtractByKey("a")
							if !ok {
								ctx.Reporter().Fatal("var not found")
							}
							ctx.Reporter().Logf("teardown a %v", v)
						}
					},
				},
				{
					name: "b",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup b")
						ctx = ctx.WithVars(map[string]int{"b": 2})
						return ctx, func(ctx *plugin.Context) {
							v, ok := ctx.Vars().ExtractByKey("b")
							if !ok {
								ctx.Reporter().Fatal("var not found")
							}
							ctx.Reporter().Logf("teardown b %v", v)
						}
					},
				},
				{
					name: "c",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup c")
						ctx = ctx.WithVars(map[string]int{"c": 3})
						return ctx, func(ctx *plugin.Context) {
							v, ok := ctx.Vars().ExtractByKey("c")
							if !ok {
								ctx.Reporter().Fatal("var not found")
							}
							ctx.Reporter().Logf("teardown c %v", v)
						}
					},
				},
			},
			failed: false,
			expect: `
=== RUN   setup
=== RUN   setup/a
=== RUN   setup/b
=== RUN   setup/c
--- PASS: setup (0.00s)
    --- PASS: setup/a (0.00s)
            setup a
    --- PASS: setup/b (0.00s)
            setup b
    --- PASS: setup/c (0.00s)
            setup c
PASS
ok  	setup	0.000s
=== RUN   teardown
=== RUN   teardown/c
=== RUN   teardown/b
=== RUN   teardown/a
--- PASS: teardown (0.00s)
    --- PASS: teardown/c (0.00s)
            teardown c 3
    --- PASS: teardown/b (0.00s)
            teardown b 2
    --- PASS: teardown/a (0.00s)
            teardown a 1
PASS
ok  	teardown	0.000s
`,
		},
		"setup failed": {
			setups: setupFuncList{
				{
					name: "a",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup a")
						ctx = ctx.WithVars(map[string]int{"a": 1})
						return ctx, func(ctx *plugin.Context) {
							v, ok := ctx.Vars().ExtractByKey("a")
							if !ok {
								ctx.Reporter().Fatal("var not found")
							}
							ctx.Reporter().Logf("teardown a %v", v)
						}
					},
				},
				{
					name: "b",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Fatal("setup b failed")
						return ctx, nil
					},
				},
				{
					name: "c",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup c")
						ctx = ctx.WithVars(map[string]int{"c": 3})
						return ctx, func(ctx *plugin.Context) {
							v, ok := ctx.Vars().ExtractByKey("c")
							if !ok {
								ctx.Reporter().Fatal("var not found")
							}
							ctx.Reporter().Logf("teardown c %v", v)
						}
					},
				},
			},
			failed: true,
			expect: `
=== RUN   setup
=== RUN   setup/a
=== RUN   setup/b
--- FAIL: setup (0.00s)
    --- PASS: setup/a (0.00s)
            setup a
    --- FAIL: setup/b (0.00s)
            setup b failed
FAIL
FAIL	setup	0.000s
FAIL
=== RUN   teardown
=== RUN   teardown/a
--- PASS: teardown (0.00s)
    --- PASS: teardown/a (0.00s)
            teardown a 1
PASS
ok  	teardown	0.000s
`,
		},
		"teardown failed": {
			setups: setupFuncList{
				{
					name: "a",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup a")
						ctx = ctx.WithVars(map[string]int{"a": 1})
						return ctx, func(ctx *plugin.Context) {
							v, ok := ctx.Vars().ExtractByKey("a")
							if !ok {
								ctx.Reporter().Fatal("var not found")
							}
							ctx.Reporter().Logf("teardown a %v", v)
						}
					},
				},
				{
					name: "b",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup b")
						return ctx, func(ctx *plugin.Context) {
							v, ok := ctx.Vars().ExtractByKey("b")
							if !ok {
								ctx.Reporter().Fatal("var not found")
							}
							ctx.Reporter().Logf("teardown b %v", v)
						}
					},
				},
				{
					name: "c",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup c")
						ctx = ctx.WithVars(map[string]int{"c": 3})
						return ctx, func(ctx *plugin.Context) {
							v, ok := ctx.Vars().ExtractByKey("c")
							if !ok {
								ctx.Reporter().Fatal("var not found")
							}
							ctx.Reporter().Logf("teardown c %v", v)
						}
					},
				},
			},
			failed: true,
			expect: `
=== RUN   setup
=== RUN   setup/a
=== RUN   setup/b
=== RUN   setup/c
--- PASS: setup (0.00s)
    --- PASS: setup/a (0.00s)
            setup a
    --- PASS: setup/b (0.00s)
            setup b
    --- PASS: setup/c (0.00s)
            setup c
PASS
ok  	setup	0.000s
=== RUN   teardown
=== RUN   teardown/c
=== RUN   teardown/b
=== RUN   teardown/a
--- FAIL: teardown (0.00s)
    --- PASS: teardown/c (0.00s)
            teardown c 3
    --- FAIL: teardown/b (0.00s)
            var not found
    --- PASS: teardown/a (0.00s)
            teardown a 1
FAIL
FAIL	teardown	0.000s
FAIL
`,
		},
		"setup returns nil context": {
			setups: setupFuncList{
				{
					name: "a",
					f: func(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
						ctx.Reporter().Log("setup a")
						return nil, nil
					},
				},
			},
			failed: false,
			expect: `
=== RUN   setup
=== RUN   setup/a
--- PASS: setup (0.00s)
    --- PASS: setup/a (0.00s)
            setup a
PASS
ok  	setup	0.000s
`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			failed := !reporter.Run(func(r reporter.Reporter) {
				ctx := context.New(r)
				ctx, teardown := test.setups.setup(ctx)
				teardown(ctx)
			}, reporter.WithWriter(&b), reporter.WithVerboseLog())
			if failed != test.failed {
				t.Fatalf("expect failed %t but got %t", test.failed, failed)
			}
			if test.expect != "" {
				if got, expect := testutil.ReplaceOutput(b.String()), strings.TrimPrefix(test.expect, "\n"); got != expect {
					dmp := diffmatchpatch.New()
					diffs := dmp.DiffMain(expect, got, false)
					t.Errorf("stdout differs:\n%s", dmp.DiffPrettyText(diffs))
				}
			}
		})
	}
}
