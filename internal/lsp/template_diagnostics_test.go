package lsp

import (
	"context"
	goast "go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/scenarigo/scenarigo/template"
)

// TestAssertCandidatesMatchTemplateHelpers compares the hand-written list of
// {{assert.x}} helpers with the keys assert/template_helpers.go resolves.
// The list there is the switch of an unexported method, so it is read from
// the source instead of adding a public API.
func TestAssertCandidatesMatchTemplateHelpers(t *testing.T) {
	cases := switchCases(t, filepath.Join("..", "..", "assert", "template_helpers.go"), "assertions", "ExtractByKey")
	if len(cases) == 0 {
		t.Fatal("no cases found in assertions.ExtractByKey")
	}
	var names []string
	for _, c := range assertCandidates() {
		names = append(names, c.name)
	}
	for _, c := range cases {
		if !slices.Contains(names, c) {
			t.Errorf("assert.%s is resolved by assert/template_helpers.go but missing from assertCandidates", c)
		}
	}
	for _, n := range names {
		if !slices.Contains(cases, n) {
			t.Errorf("assertCandidates lists assert.%s, which assert/template_helpers.go does not resolve", n)
		}
	}
}

// TestTemplateContextRootsMatchExtract compares templateContextRoots with the
// names context/extract.go resolves.
func TestTemplateContextRootsMatchExtract(t *testing.T) {
	path := filepath.Join("..", "..", "context", "extract.go")
	consts := stringConsts(t, path)
	cases := switchCases(t, path, "Context", "ExtractByKey")
	var roots []string
	for _, c := range cases {
		v, ok := consts[c]
		if !ok {
			t.Fatalf("case %s of Context.ExtractByKey is not a string constant", c)
		}
		roots = append(roots, v)
	}
	if len(roots) == 0 {
		t.Fatal("no cases found in Context.ExtractByKey")
	}
	slices.Sort(roots)
	got := slices.Sorted(slices.Values(templateContextRoots))
	if !slices.Equal(got, roots) {
		t.Errorf("templateContextRoots = %v, context/extract.go resolves %v", got, roots)
	}
}

// TestTemplateBuiltinsAreDefined checks that the template package resolves
// every name of templateBuiltins without any data, and nothing else.
func TestTemplateBuiltinsAreDefined(t *testing.T) {
	for _, name := range append(slices.Clone(templateBuiltins), "notABuiltin") {
		v, err := template.Execute(context.Background(), "{{defined("+name+")}}", nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := name != "notABuiltin"; v != want {
			t.Errorf("defined(%s) = %v, want %v", name, v, want)
		}
	}
}

// TestTemplateCompletionNamesAreKnown checks that completion never offers a
// top-level name that the diagnostics would report as unknown.
func TestTemplateCompletionNamesAreKnown(t *testing.T) {
	items := (&Server{}).completeTemplate(nil, "")
	if len(items) == 0 {
		t.Fatal("no top-level template completions")
	}
	for _, item := range items {
		if !slices.Contains(templateContextRoots, item.Label) && !slices.Contains(templateBuiltins, item.Label) {
			t.Errorf("completion offers %q, which the diagnostics do not know", item.Label)
		}
	}
}

// switchCases returns the case expressions of the switch statements in the
// method recv.name of a Go file: string literals by value, identifiers by
// name.
func switchCases(t *testing.T, path, recv, name string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var cases []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*goast.FuncDecl)
		if !ok || fn.Name.Name != name || fn.Recv == nil || len(fn.Recv.List) != 1 || receiverName(fn.Recv.List[0].Type) != recv {
			continue
		}
		goast.Inspect(fn.Body, func(n goast.Node) bool {
			cc, ok := n.(*goast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				switch e := e.(type) {
				case *goast.BasicLit:
					if s, err := strconv.Unquote(e.Value); err == nil {
						cases = append(cases, s)
					}
				case *goast.Ident:
					cases = append(cases, e.Name)
				}
			}
			return true
		})
	}
	return cases
}

func receiverName(expr goast.Expr) string {
	if star, ok := expr.(*goast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*goast.Ident); ok {
		return id.Name
	}
	return ""
}

// stringConsts returns the string constants declared in a Go file.
func stringConsts(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	consts := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*goast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*goast.ValueSpec)
			for i, n := range vs.Names {
				if i < len(vs.Values) {
					if lit, ok := vs.Values[i].(*goast.BasicLit); ok {
						if s, err := strconv.Unquote(lit.Value); err == nil {
							consts[n.Name] = s
						}
					}
				}
			}
		}
	}
	return consts
}
