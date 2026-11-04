package lsp

import (
	"context"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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

// TestTemplateBuiltinsMatchTemplatePackage compares templateBuiltins with the
// names the template package resolves without any data. The candidates are
// the lower-case string literals of the files that define them (the
// functions map, the type function and the registered types of
// template/val): every candidate that {{defined(x)}} accepts must be a
// built-in, and every built-in must be accepted.
func TestTemplateBuiltinsMatchTemplatePackage(t *testing.T) {
	files := []string{filepath.Join("..", "..", "template", "function.go"), filepath.Join("..", "..", "template", "type.go")}
	valFiles, err := filepath.Glob(filepath.Join("..", "..", "template", "val", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range valFiles {
		if !strings.HasSuffix(f, "_test.go") {
			files = append(files, f)
		}
	}
	keywords := []string{"nil", "null", "true", "false", "defined"}
	candidates := map[string]bool{}
	for _, f := range files {
		for _, lit := range stringLiterals(t, f) {
			if isLowerIdent(lit) && !slices.Contains(keywords, lit) {
				candidates[lit] = true
			}
		}
	}
	defined := func(name string) bool {
		v, err := template.Execute(context.Background(), "{{defined("+name+")}}", nil)
		if err != nil {
			t.Fatalf("defined(%s): %v", name, err)
		}
		return v == true
	}
	for name := range candidates {
		if defined(name) && !slices.Contains(templateBuiltins, name) {
			t.Errorf("the template package resolves %q, which templateBuiltins lacks", name)
		}
	}
	for _, name := range templateBuiltins {
		if !defined(name) {
			t.Errorf("templateBuiltins lists %q, which the template package does not resolve", name)
		}
	}
}

func isLowerIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if (r < 'a' || r > 'z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// stringLiterals returns the string literals of a Go file.
func stringLiterals(t *testing.T, path string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	goast.Inspect(f, func(n goast.Node) bool {
		if lit, ok := n.(*goast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil {
				out = append(out, s)
			}
		}
		return true
	})
	return out
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

// TestDiagnosticsScaleLinearly checks that the diagnostics of a file take
// time in proportion to its size, for the shapes of large files: many
// documents, many steps, steps that bind variables, include steps, null
// values (checked for ytt annotations), and long lines of plain values, of
// templates, and of diagnostics, also with a non-ASCII character. Each case
// measures publishing the diagnostics of a parsed document.
func TestDiagnosticsScaleLinearly(t *testing.T) {
	step := func(b *strings.Builder, i int) {
		fmt.Fprintf(b, "  - id: s%d\n    protocol: http\n    request:\n      method: GET\n      url: \"http://{{vars.host}}/{{vars.missing}}\"\n    expect:\n      code: OK\n      body:\n        id: '{{steps.s%d.result}}'\n", i, i)
	}
	nonASCII := func(s string) string { return strings.Replace(s, `"key0"`, `"キー0"`, 1) }
	cases := []struct {
		name  string
		size  int
		text  func(n int) string
		files func(n int) map[string]string // next to the document
	}{
		{name: "documents", size: 250, text: func(n int) string {
			var b strings.Builder
			for i := range n {
				b.WriteString("---\nschemaVersion: scenario/v1\nvars:\n  host: example.com\nsteps:\n")
				step(&b, i)
			}
			return b.String()
		}},
		{name: "steps", size: 250, text: func(n int) string {
			var b strings.Builder
			b.WriteString("schemaVersion: scenario/v1\nvars:\n  host: example.com\nsteps:\n")
			for i := range n {
				step(&b, i)
			}
			return b.String()
		}},
		{name: "binds", size: 2000, text: func(n int) string {
			var b strings.Builder
			b.WriteString("schemaVersion: scenario/v1\nsteps:\n")
			for i := range n {
				fmt.Fprintf(&b, "  - id: s%d\n    protocol: http\n    request:\n      method: GET\n      url: \"http://x/{{vars.v%d}}\"\n    bind:\n      vars:\n        v%d: '{{response.body}}'\n", i, i, i+1)
			}
			return b.String()
		}},
		{name: "include steps", size: 1000, text: func(n int) string {
			var b strings.Builder
			b.WriteString("schemaVersion: scenario/v1\nsteps:\n")
			for i := range n {
				fmt.Fprintf(&b, "  - id: s%d\n    include: missing%d.yaml\n    bind:\n      vars:\n        v%d: '{{vars.x}}'\n", i, i, i)
			}
			return b.String()
		}},
		// The included file grows with the document: its vars are looked
		// up in the bind of every include step.
		{name: "include steps of one file", size: 250, text: func(n int) string {
			var b strings.Builder
			b.WriteString("schemaVersion: scenario/v1\nsteps:\n")
			for i := range n {
				fmt.Fprintf(&b, "  - include: common.yaml\n    bind:\n      vars:\n        v%d: '{{vars.c%d}}'\n", i, i)
			}
			return b.String()
		}, files: func(n int) map[string]string {
			var b strings.Builder
			b.WriteString("schemaVersion: scenario/v1\nvars:\n")
			for i := range n {
				fmt.Fprintf(&b, "  c%d: %d\n", i, i)
			}
			b.WriteString("steps:\n  - protocol: http\n    request:\n      method: GET\n      url: http://x\n")
			return map[string]string{"common.yaml": b.String()}
		}},
		{name: "nulls", size: 250, text: func(n int) string {
			var b strings.Builder
			b.WriteString("schemaVersion: scenario/v1\nsteps:\n")
			for i := range n {
				fmt.Fprintf(&b, "  - title: s%d\n    description:\n    protocol: http\n    request:\n      method: GET\n      url: http://x\n      header:\n      body:\n    expect:\n      body:\n", i)
			}
			return b.String()
		}},
		{name: "long line of nulls", size: 500, text: func(n int) string {
			steps := make([]string, n)
			for i := range steps {
				steps[i] = fmt.Sprintf(`{title: s%d, description: null, protocol: http, request: {method: GET, url: "http://x"}}`, i)
			}
			return "schemaVersion: scenario/v1\nsteps: [" + strings.Join(steps, ", ") + "]\n"
		}},
		{name: "long line", size: 2500, text: func(n int) string { return longLine(n, "\"value %d\"") }},
		{name: "long line of templates", size: 1000, text: func(n int) string { return longLine(n, "\"{{vars.host}}/%d\"") }},
		// Each template gets an information diagnostic on the line.
		{name: "long line of diagnostics", size: 1000, text: func(n int) string { return longLine(n, "\"{{vars.v%d}}\"") }},
		{name: "long non-ASCII line of templates", size: 1000, text: func(n int) string { return nonASCII(longLine(n, "\"{{vars.host}}/%d\"")) }},
		{name: "long non-ASCII line of diagnostics", size: 1000, text: func(n int) string { return nonASCII(longLine(n, "\"{{vars.v%d}}\"")) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertLinear(t, c.size, func(n int) time.Duration {
				s := NewServer()
				s.writer = io.Discard
				s.logger = log.New(io.Discard, "", 0)
				dir := t.TempDir()
				if c.files != nil {
					for name, text := range c.files(n) {
						if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
							t.Fatal(err)
						}
					}
				}
				uri := pathToURI(filepath.Join(dir, "test.yaml"))
				s.docs.Open(uri, 1, c.text(n))
				start := time.Now()
				s.publishDiagnostics(uri)
				return time.Since(start)
			})
		})
	}
}

// longLine returns a scenario whose expected body is a flow mapping of n
// keys on one line, the values formatted with their index.
func longLine(n int, value string) string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("\"key%d\": "+value, i, i)
	}
	return "schemaVersion: scenario/v1\nvars:\n  host: example.com\nsteps:\n  - protocol: http\n    request:\n      method: GET\n      url: http://x\n    expect:\n      body: {" + strings.Join(keys, ", ") + "}\n"
}

// assertLinear compares eight runs at a size with one run at eight times
// the size, and fails when the larger run takes three times as long or
// more: linear code takes about as long, quadratic code about eight times as
// long. A ratio rather than a time limit keeps the test valid on a slow
// machine, and both measurements last about as long, so that other work on
// the machine slows them alike. After a warm-up run, up to three rounds are
// measured, keeping the fastest of each, until the ratio is under the limit.
// Timings under the race detector are skipped.
func assertLinear(t *testing.T, size int, run func(n int) time.Duration) {
	t.Helper()
	if raceSlowdown > 1 {
		t.Skip("timings under the race detector")
	}
	run(size)
	small, large := time.Duration(math.MaxInt64), time.Duration(math.MaxInt64)
	for range 3 {
		var sum time.Duration
		for range 8 {
			sum += run(size)
		}
		small, large = min(small, sum), min(large, run(8*size))
		if large < 3*small || large < 100*time.Millisecond {
			t.Logf("8 × %d: %v, %d: %v", size, small, 8*size, large)
			return
		}
	}
	t.Errorf("%v for 8 × %d, %v for %d: slower than linear", small, size, large, 8*size)
}
