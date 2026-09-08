package plugin

import (
	"errors"
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// migrationModule returns a tidied go.mod and go.sum for a plugin module that
// depends on this checkout of scenarigo, so that test plugins type-check
// against the real extractor signatures.
func migrationModule(t *testing.T) ([]byte, []byte) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	create(t, filepath.Join(dir, "go.mod"), "module plugins/migrate\n\ngo "+strings.TrimPrefix(goVer, "go")+"\n\nrequire github.com/scenarigo/scenarigo v0.0.0\n\nreplace github.com/scenarigo/scenarigo => "+root+"\n")
	create(t, filepath.Join(dir, "main.go"), "package main\n\nimport _ \"github.com/scenarigo/scenarigo/plugin\"\n")
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = commandEnv([]string{"GOWORK=off"})
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %s\n%s", err, out)
	}
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	gosum, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	return gomod, gosum
}

func runMigration(t *testing.T, dir string, skipFiles ...string) (*migrationResult, error) {
	t.Helper()
	return migrateExtractorCalls(t.Context(), &migration{
		dir:       dir,
		env:       commandEnv([]string{"GOWORK=off"}),
		skipFiles: skipFiles,
	})
}

const pluginImport = "import \"github.com/scenarigo/scenarigo/plugin\"\n"

func TestMigrateExtractorCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks a module against the scenarigo checkout")
	}
	gomod, gosum := migrationModule(t)

	tests := map[string]struct {
		files         map[string]string
		expectFiles   map[string]string // rewritten content; a missing entry means untouched
		expectChanges []string          // substrings, one per change
		expectManual  []string          // substrings, one per manual warning
	}{
		"one-arg call with !ok": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	v, ok := ctx.Vars().ExtractByKey("k")
	if !ok {
		return nil
	}
	return v
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	v, err := ctx.Vars().ExtractByKey(ctx.RequestContext(), "k")
	if err != nil {
		return nil
	}
	return v
}
`},
			expectChanges: []string{`main.go:6:22: ExtractByKey("k") => ExtractByKey(ctx.RequestContext(), "k"), ok => err`},
		},
		"every scenarigo receiver": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) []bool {
	_, a := ctx.Plugins().ExtractByKey("p")
	_, b := ctx.Steps().ExtractByKey("s")
	_, c := ctx.ExtractByKey("vars")
	return []bool{a, b, c}
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) []bool {
	_, err := ctx.Plugins().ExtractByKey(ctx.RequestContext(), "p")
	_, b := ctx.Steps().ExtractByKey(ctx.RequestContext(), "s")
	_, c := ctx.ExtractByKey(ctx.RequestContext(), "vars")
	return []bool{err == nil, b == nil, c == nil}
}
`},
			expectChanges: []string{"a => err", "b is now an error", "c is now an error"},
		},
		"if init and boolean expressions": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context, other bool) (any, bool) {
	if v, ok := ctx.Vars().ExtractByKey("k"); ok {
		return v, true
	}
	v, ok := ctx.Vars().ExtractByKey("k")
	found := ok && other
	return v, found
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context, other bool) (any, bool) {
	if v, err := ctx.Vars().ExtractByKey(ctx.RequestContext(), "k"); err == nil {
		return v, true
	}
	v, err := ctx.Vars().ExtractByKey(ctx.RequestContext(), "k")
	found := (err == nil) && other
	return v, found
}
`},
			// The first err lives in the if statement's scope and is out of
			// scope by the second call, so both variables can take the name.
			expectChanges: []string{"ok => err", "ok => err"},
		},
		"second result discarded": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	v, _ := ctx.Vars().ExtractByKey("k")
	ctx.Vars().ExtractByKey("unused")
	return v
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	v, _ := ctx.Vars().ExtractByKey(ctx.RequestContext(), "k")
	ctx.Vars().ExtractByKey(ctx.RequestContext(), "unused")
	return v
}
`},
			expectChanges: []string{`ExtractByKey("k") => ExtractByKey(ctx.RequestContext(), "k")`, `ExtractByKey("unused") => ExtractByKey(ctx.RequestContext(), "unused")`},
		},
		"err already in scope keeps the name": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
import "os"

func F(ctx *plugin.Context) (any, error) {
	_, err := os.Stat(".")
	v, ok := ctx.Vars().ExtractByKey("k")
	if !ok {
		return nil, err
	}
	return v, nil
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\n" + pluginImport + `
import "os"

func F(ctx *plugin.Context) (any, error) {
	_, err := os.Stat(".")
	v, ok := ctx.Vars().ExtractByKey(ctx.RequestContext(), "k")
	if ok != nil {
		return nil, err
	}
	return v, nil
}
`},
			expectChanges: []string{"ok is now an error"},
		},
		"assignment to an existing variable is manual": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) bool {
	var ok bool
	_, ok = ctx.Vars().ExtractByKey("k")
	return ok
}
`},
			expectManual: []string{"main.go:7:21: ExtractByKey cannot be migrated automatically because its second result is assigned to an existing variable"},
		},
		"variable written later is manual": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) bool {
	_, ok := ctx.Vars().ExtractByKey("k")
	ok = true
	return ok
}
`},
			expectManual: []string{"because its second result is assigned to later"},
		},
		"passthrough is manual": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) (any, bool) {
	return ctx.Vars().ExtractByKey("k")
}
`},
			expectManual: []string{"main.go:6:20: ExtractByKey cannot be migrated automatically because its results are not bound by a two-value assignment"},
		},
		"secrets with a context and !ok": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	v, ok := ctx.Secrets().ExtractByKey(ctx.RequestContext(), "k")
	if !ok {
		return nil
	}
	return v
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	v, err := ctx.Secrets().ExtractByKey(ctx.RequestContext(), "k")
	if err != nil {
		return nil
	}
	return v
}
`},
			expectChanges: []string{`main.go:6:25: ExtractByKey(ctx.RequestContext(), "k"), ok => err`},
		},
		"secrets already migrated": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	v, err := ctx.Secrets().ExtractByKey(ctx.RequestContext(), "k")
	if err != nil {
		return nil
	}
	return v
}
`},
		},
		"secrets already migrated next to a v1 call": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	s, err := ctx.Secrets().ExtractByKey(ctx.RequestContext(), "token")
	v, ok := ctx.Vars().ExtractByKey("k")
	if err != nil || !ok {
		return nil
	}
	return []any{s, v}
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	s, err := ctx.Secrets().ExtractByKey(ctx.RequestContext(), "token")
	v, ok := ctx.Vars().ExtractByKey(ctx.RequestContext(), "k")
	if err != nil || (ok != nil) {
		return nil
	}
	return []any{s, v}
}
`},
			expectChanges: []string{`main.go:7:22: ExtractByKey("k") => ExtractByKey(ctx.RequestContext(), "k"), ok is now an error`},
		},
		"secrets already migrated with an unrelated error in the same statement": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	v, err := ctx.Secrets().ExtractByKey(ctx.RequestContext(), "k")
	if err != nil || nope {
		return nil
	}
	return v
}
`},
		},
		"secrets returned as is": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) (any, bool) {
	return ctx.Secrets().ExtractByKey(ctx.RequestContext(), "k")
}
`},
			expectManual: []string{"main.go:6:23: ExtractByKey cannot be migrated automatically because its results are not bound by a two-value assignment"},
		},
		"secrets returned as is in the v2 shape": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) (any, error) {
	return ctx.Secrets().ExtractByKey(ctx.RequestContext(), "k")
}
`},
		},
		"secrets with a context assigned to later": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	v, ok := ctx.Secrets().ExtractByKey(ctx.RequestContext(), "k")
	ok = true
	_ = ok
	return v
}
`},
			expectManual: []string{"main.go:6:25: ExtractByKey cannot be migrated automatically because its second result is assigned to later"},
		},
		"only the packages the build compiles are rewritten": {
			files: map[string]string{
				"main.go": "package main\n\n" + `import (
	"github.com/scenarigo/scenarigo/plugin"

	"plugins/migrate/sub"
)

func F(ctx *plugin.Context) any { return sub.G(ctx) }
`,
				"sub/sub.go": "package sub\n\n" + pluginImport + `
func G(ctx *plugin.Context) any {
	v, ok := ctx.Vars().ExtractByKey("k")
	if !ok {
		return nil
	}
	return v
}
`,
				"other/other.go": "package other\n\n" + pluginImport + `
func H(ctx *plugin.Context) any {
	v, ok := ctx.Vars().ExtractByKey("k")
	if !ok {
		return nil
	}
	return v
}
`,
				// A package the build never compiles may even fail to parse.
				"broken/broken.go": "package broken\n\nfunc broken( {\n",
			},
			expectFiles: map[string]string{"sub/sub.go": "package sub\n\n" + pluginImport + `
func G(ctx *plugin.Context) any {
	v, err := ctx.Vars().ExtractByKey(ctx.RequestContext(), "k")
	if err != nil {
		return nil
	}
	return v
}
`},
			expectChanges: []string{`sub/sub.go:6:22: ExtractByKey("k") => ExtractByKey(ctx.RequestContext(), "k"), ok => err`},
		},
		"no free name for the context import": {
			files: map[string]string{"main.go": "package main\n\n" + `import (
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/plugin"
)

var gocontext, gocontext2, gocontext3 int

func F(ctx *plugin.Context, vars context.Vars) []bool {
	_, a := vars.ExtractByKey("k")
	_, b := ctx.Vars().ExtractByKey("k")
	return []bool{a, b}
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\n" + `import (
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/plugin"
)

var gocontext, gocontext2, gocontext3 int

func F(ctx *plugin.Context, vars context.Vars) []bool {
	_, a := vars.ExtractByKey("k")
	_, err := ctx.Vars().ExtractByKey(ctx.RequestContext(), "k")
	return []bool{a, err == nil}
}
`},
			expectChanges: []string{`main.go:12:21: ExtractByKey("k") => ExtractByKey(ctx.RequestContext(), "k"), b => err`},
			expectManual:  []string{"main.go:11:15: ExtractByKey cannot be migrated automatically because the file has no free name to import the context package as"},
		},
		"secrets with an unused second result": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) any {
	v, ok := ctx.Secrets().ExtractByKey(ctx.RequestContext(), "k")
	return v
}
`},
		},
		"the plugin's own extractor is left alone": {
			files: map[string]string{"main.go": "package main\n\n" + pluginImport + `
type own struct{}

func (own) ExtractByKey(key string) (any, bool) { return key, true }

func F(ctx *plugin.Context) (any, any, bool) {
	a, _ := own{}.ExtractByKey(
		"mine",
	)
	b, _ := own{}.ExtractByKey("mine"); c, ok := ctx.Vars().ExtractByKey("theirs")
	return a, b, ok && c != nil
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\n" + pluginImport + `
type own struct{}

func (own) ExtractByKey(key string) (any, bool) { return key, true }

func F(ctx *plugin.Context) (any, any, bool) {
	a, _ := own{}.ExtractByKey(
		"mine",
	)
	b, _ := own{}.ExtractByKey("mine"); c, err := ctx.Vars().ExtractByKey(ctx.RequestContext(), "theirs")
	return a, b, (err == nil) && c != nil
}
`},
			expectChanges: []string{`main.go:13:58: ExtractByKey("theirs") => ExtractByKey(ctx.RequestContext(), "theirs"), ok => err`},
		},
		"receiver without a scenarigo context": {
			files: map[string]string{"main.go": "package main\n\n" + `import (
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/plugin"
)

type holder struct{ ctx *plugin.Context }

func get() *plugin.Context { return nil }

func F(vars context.Vars, h holder) []bool {
	_, a := vars.ExtractByKey("k") // comment kept
	_, b := h.ctx.Vars().ExtractByKey("k")
	_, c := get().Vars().ExtractByKey("k")
	return []bool{a, b, c}
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\nimport gocontext \"context\"\n\n" + `import (
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/plugin"
)

type holder struct{ ctx *plugin.Context }

func get() *plugin.Context { return nil }

func F(vars context.Vars, h holder) []bool {
	_, err := vars.ExtractByKey(gocontext.Background(), "k") // comment kept
	_, b := h.ctx.Vars().ExtractByKey(h.ctx.RequestContext(), "k")
	_, c := get().Vars().ExtractByKey(gocontext.Background(), "k")
	return []bool{err == nil, b == nil, c == nil}
}
`},
			expectChanges: []string{"main.go:13:15: ExtractByKey(\"k\") => ExtractByKey(gocontext.Background(), \"k\"), a => err", "main.go:14:23: ExtractByKey(\"k\") => ExtractByKey(h.ctx.RequestContext(), \"k\"), b is now an error", "main.go:15:23: ExtractByKey(\"k\") => ExtractByKey(gocontext.Background(), \"k\"), c is now an error"},
		},
		"standard context already imported": {
			files: map[string]string{"main.go": "package main\n\n" + `import (
	"context"

	scenarigo "github.com/scenarigo/scenarigo/context"
)

var _ context.Context

func F(vars scenarigo.Vars) bool {
	_, ok := vars.ExtractByKey("k")
	return ok
}
`},
			expectFiles: map[string]string{"main.go": "package main\n\n" + `import (
	"context"

	scenarigo "github.com/scenarigo/scenarigo/context"
)

var _ context.Context

func F(vars scenarigo.Vars) bool {
	_, err := vars.ExtractByKey(context.Background(), "k")
	return err == nil
}
`},
			expectChanges: []string{"context.Background()"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			create(t, filepath.Join(dir, "go.mod"), string(gomod))
			create(t, filepath.Join(dir, "go.sum"), string(gosum))
			for p, content := range test.files {
				create(t, filepath.Join(dir, p), content)
			}
			res, err := runMigration(t, dir)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if len(res.changes) != len(test.expectChanges) {
				t.Errorf("expected %d changes but got %d: %q", len(test.expectChanges), len(res.changes), res.changes)
			}
			for i, expect := range test.expectChanges {
				if i < len(res.changes) && !strings.Contains(res.changes[i], expect) {
					t.Errorf("change %d: expected %q in %q", i, expect, res.changes[i])
				}
			}
			if len(res.manual) != len(test.expectManual) {
				t.Errorf("expected %d manual warnings but got %d: %q", len(test.expectManual), len(res.manual), res.manual)
			}
			for i, expect := range test.expectManual {
				if i < len(res.manual) && !strings.Contains(res.manual[i], expect) {
					t.Errorf("manual %d: expected %q in %q", i, expect, res.manual[i])
				}
			}
			for p, content := range test.files {
				b, err := os.ReadFile(filepath.Join(dir, p))
				if err != nil {
					t.Fatal(err)
				}
				expect, rewritten := test.expectFiles[p]
				if !rewritten {
					expect = content
				}
				if got := string(b); got != expect {
					t.Errorf("%s: unexpected content:\n--- expect ---\n%s\n--- got ---\n%s", p, expect, got)
				}
				if _, ok := res.originals[filepath.Join(dir, p)]; ok != rewritten {
					t.Errorf("%s: recorded as original: %t, rewritten: %t", p, ok, rewritten)
				}
				if rewritten && string(res.originals[filepath.Join(dir, p)].content) != content {
					t.Errorf("%s: the recorded original differs from the input", p)
				}
			}
		})
	}
}

func TestMigrateExtractorCalls_Files(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks a module against the scenarigo checkout")
	}
	gomod, gosum := migrationModule(t)
	src := "package main\n\n" + pluginImport + `
func F(ctx *plugin.Context) bool {
	_, ok := ctx.Vars().ExtractByKey("k")
	return ok
}
`
	newDir := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		create(t, filepath.Join(dir, "go.mod"), string(gomod))
		create(t, filepath.Join(dir, "go.sum"), string(gosum))
		return dir
	}

	t.Run("skipped and out-of-package files are untouched", func(t *testing.T) {
		dir := newDir(t)
		create(t, filepath.Join(dir, "main.go"), src)
		generated := filepath.Join(dir, "generated.go")
		create(t, generated, strings.Replace(src, "func F(", "func G(", 1))
		create(t, filepath.Join(dir, "main_test.go"), strings.Replace(src, "func F(", "func H(", 1))
		res, err := runMigration(t, dir, generated)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if len(res.changes) != 1 {
			t.Fatalf("expected 1 change but got %q", res.changes)
		}
		for _, name := range []string{"generated.go", "main_test.go"} {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), "RequestContext") {
				t.Errorf("%s was rewritten", name)
			}
		}
	})

	t.Run("a single-file build target limits the rewrite to that file", func(t *testing.T) {
		dir := newDir(t)
		create(t, filepath.Join(dir, "main.go"), src)
		create(t, filepath.Join(dir, "helper.go"), strings.Replace(src, "func F(", "func G(", 1))
		res, err := migrateExtractorCalls(t.Context(), &migration{
			dir:    dir,
			env:    commandEnv([]string{"GOWORK=off"}),
			target: filepath.Join(dir, "main.go"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if len(res.changes) != 1 || !strings.HasPrefix(res.changes[0], "main.go:") {
			t.Fatalf("expected 1 change in main.go but got %q", res.changes)
		}
		b, err := os.ReadFile(filepath.Join(dir, "helper.go"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "RequestContext") {
			t.Error("helper.go was rewritten")
		}
	})

	t.Run("a plugin in a subdirectory of its module is migrated", func(t *testing.T) {
		// The build compiles ./plugin, not the module root, so the closure has
		// to start there. Starting at the root found no package to start from.
		dir := newDir(t)
		create(t, filepath.Join(dir, "plugin", "main.go"), src)
		create(t, filepath.Join(dir, "dep", "dep.go"), "package dep\n\n"+pluginImport+`
func D(ctx *plugin.Context) bool {
	_, ok := ctx.Vars().ExtractByKey("k")
	return ok
}
`)
		create(t, filepath.Join(dir, "plugin", "use.go"), "package main\n\nimport _ \"plugins/migrate/dep\"\n")
		res, err := migrateExtractorCalls(t.Context(), &migration{
			dir:    dir,
			pkgDir: filepath.Join(dir, "plugin"),
			env:    commandEnv([]string{"GOWORK=off"}),
		})
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if len(res.changes) != 2 {
			t.Fatalf("expected the plugin package and its dependency to be rewritten but got %q", res.changes)
		}
	})

	t.Run("a single-file build follows the imports of that file only", func(t *testing.T) {
		// The package the build names is only partly compiled, so a package
		// that only an uncompiled file of it imports is not part of the build.
		dir := newDir(t)
		dep := "package %s\n\n" + pluginImport + `
func D(ctx *plugin.Context) bool {
	_, ok := ctx.Vars().ExtractByKey("k")
	return ok
}
`
		create(t, filepath.Join(dir, "a", "a.go"), fmt.Sprintf(dep, "a"))
		create(t, filepath.Join(dir, "b", "b.go"), fmt.Sprintf(dep, "b"))
		create(t, filepath.Join(dir, "main.go"), "package main\n\nimport _ \"plugins/migrate/a\"\n\n"+strings.TrimPrefix(src, "package main\n"))
		create(t, filepath.Join(dir, "helper.go"), "package main\n\nimport _ \"plugins/migrate/b\"\n")
		res, err := migrateExtractorCalls(t.Context(), &migration{
			dir:    dir,
			pkgDir: dir,
			env:    commandEnv([]string{"GOWORK=off"}),
			target: filepath.Join(dir, "main.go"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		for _, c := range res.changes {
			if strings.HasPrefix(c, filepath.Join("b", "b.go")) {
				t.Errorf("a package only an uncompiled file imports was rewritten: %q", res.changes)
			}
		}
		var sawA bool
		for _, c := range res.changes {
			if strings.HasPrefix(c, filepath.Join("a", "a.go")) {
				sawA = true
			}
		}
		if !sawA {
			t.Errorf("the package the compiled file imports was not rewritten: %q", res.changes)
		}
	})

	t.Run("a dependency the ... pattern skips is still migrated", func(t *testing.T) {
		// go build compiles whatever the package imports, but the "./..."
		// pattern skips directories named testdata and those starting with "_",
		// so the set has to come from the build graph rather than from a
		// pattern match.
		dir := newDir(t)
		dep := "package helper\n\n" + pluginImport + `
func H(ctx *plugin.Context) bool {
	_, ok := ctx.Vars().ExtractByKey("k")
	return ok
}
`
		create(t, filepath.Join(dir, "testdata", "helper", "h.go"), dep)
		create(t, filepath.Join(dir, "_gen", "helper", "g.go"), dep)
		create(t, filepath.Join(dir, "main.go"), "package main\n\nimport (\n\t_ \"plugins/migrate/testdata/helper\"\n\t_ \"plugins/migrate/_gen/helper\"\n)\n\n"+strings.TrimPrefix(src, "package main\n"))
		res, err := runMigration(t, dir)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		for _, want := range []string{filepath.Join("testdata", "helper", "h.go"), filepath.Join("_gen", "helper", "g.go")} {
			var found bool
			for _, c := range res.changes {
				if strings.HasPrefix(c, want+":") {
					found = true
				}
			}
			if !found {
				t.Errorf("%s was not migrated: %q", want, res.changes)
			}
		}
	})

	t.Run("nothing outside the plugin directory is a candidate", func(t *testing.T) {
		// The standard library and other modules are compiled by the build too;
		// their sources must never be rewritten.
		dir := newDir(t)
		create(t, filepath.Join(dir, "main.go"), "package main\n\nimport _ \"fmt\"\n\n"+strings.TrimPrefix(src, "package main\n"))
		deps, err := buildDeps(t.Context(), &migration{
			dir: dir,
			env: commandEnv([]string{"GOWORK=off"}),
		}, dir, ".")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		for _, d := range deps {
			if !strings.HasPrefix(d, "plugins/migrate") {
				t.Errorf("a package outside the plugin directory is a candidate: %s", d)
			}
		}
	})

	t.Run("a vendored package is not a candidate", func(t *testing.T) {
		// A vendor tree sits inside the plugin directory but holds other
		// modules' sources. go list reports its packages with a Dir under the
		// plugin directory, so the containment check alone would hand them to
		// the migration for rewriting.
		dir := t.TempDir()
		create(t, filepath.Join(dir, "go.mod"), "module plugins/migrate\n\ngo "+strings.TrimPrefix(goVer, "go")+"\n\nrequire example.com/vendored v1.0.0\n")
		create(t, filepath.Join(dir, "vendor", "modules.txt"), "# example.com/vendored v1.0.0\n## explicit; go "+strings.TrimPrefix(goVer, "go")+"\nexample.com/vendored\n")
		create(t, filepath.Join(dir, "vendor", "example.com", "vendored", "v.go"), "package vendored\n\nfunc V() {}\n")
		create(t, filepath.Join(dir, "main.go"), "package main\n\nimport _ \"example.com/vendored\"\n\nfunc main() {}\n")
		deps, err := buildDeps(t.Context(), &migration{
			dir: dir,
			env: commandEnv([]string{"GOWORK=off"}),
		}, dir, ".")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		for _, d := range deps {
			if d == "example.com/vendored" {
				t.Errorf("a vendored package is a candidate: %q", deps)
			}
		}
	})

	t.Run("the file mode is preserved", func(t *testing.T) {
		dir := newDir(t)
		path := filepath.Join(dir, "main.go")
		create(t, path, src)
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := runMigration(t, dir); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("expected mode 0600 but got %o", info.Mode().Perm())
		}
	})

	t.Run("a failed write keeps the original", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can write a read-only file")
		}
		dir := newDir(t)
		path := filepath.Join(dir, "main.go")
		create(t, path, src)
		if err := os.Chmod(path, 0o400); err != nil {
			t.Fatal(err)
		}
		res, err := runMigration(t, dir)
		if err == nil {
			t.Fatal("expected an error")
		}
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("expected a permission error but got %v", err)
		}
		orig, ok := res.originals[path]
		if !ok {
			t.Fatal("the original was not recorded")
		}
		if string(orig.content) != src {
			t.Errorf("the recorded original differs from the input")
		}
	})
}

func TestPlannerText_PositionsOutsideTheSource(t *testing.T) {
	// The positions come from the type-checked package and the bytes from the
	// file read afterwards. A file that shrank in between puts the positions
	// past its end, which used to panic before the caller could report the
	// change.
	fset := token.NewFileSet()
	f := fset.AddFile("main.go", -1, 100)
	p := &planner{
		pkg: &packages.Package{Fset: fset},
		fe:  &fileEdits{src: []byte("short")},
	}
	if got := p.text(f.Pos(0), f.Pos(90)); got != "" {
		t.Errorf("expected no text but got %q", got)
	}
	if got := p.text(f.Pos(0), f.Pos(5)); got != "short" {
		t.Errorf("expected the text within the source but got %q", got)
	}
}

func TestMigrationGoCommand(t *testing.T) {
	// The build may run a go command that is not the one on PATH (SCENARIGO_GO
	// selects it). The dependency graph and the loader have to come from that
	// same toolchain, or the migration is planned against a different build.
	t.Run("the selected command is used", func(t *testing.T) {
		m := &migration{goCmd: filepath.Join("/opt", "go1.99", "bin", "go")}
		if got := m.goCommand(); got != m.goCmd {
			t.Errorf("expected %q but got %q", m.goCmd, got)
		}
		var path string
		for _, kv := range m.loadEnv() {
			if v, ok := strings.CutPrefix(kv, "PATH="); ok {
				path = v
			}
		}
		if want := filepath.Join("/opt", "go1.99", "bin") + string(os.PathListSeparator); !strings.HasPrefix(path, want) {
			t.Errorf("expected the loader PATH to start with %q but got %q", want, path)
		}
	})

	t.Run("without one it falls back to PATH", func(t *testing.T) {
		m := &migration{env: []string{"PATH=/usr/bin"}}
		if got := m.goCommand(); got != "go" {
			t.Errorf("expected %q but got %q", "go", got)
		}
		if got := m.loadEnv(); len(got) != 1 || got[0] != "PATH=/usr/bin" {
			t.Errorf("expected the environment to be left alone but got %q", got)
		}
	})
}
