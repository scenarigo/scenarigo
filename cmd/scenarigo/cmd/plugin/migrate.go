package plugin

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// query-go v2 changed the extractor interfaces: the methods take a context and
// report absence as an error instead of a bool.
//
//	ExtractByKey(key string) (any, bool)                        // v1
//	ExtractByKey(ctx context.Context, key string) (any, error)  // v2
//
// A plugin that called one of scenarigo's extractors as a lookup API no longer
// compiles. migrateExtractorCalls rewrites such calls to the v2 shape: it
// passes a context and turns the boolean second result into an error test.

// migration describes the plugin sources to migrate and the environment the
// failed build ran in, so that the sources are type-checked exactly as the
// build saw them.
type migration struct {
	dir string
	// goCmd is the go command the build uses, which is not always the one on
	// PATH: the dependency graph has to come from the same toolchain that
	// compiles the plugin.
	goCmd string
	// pkgDir is the directory of the package the build compiles. It is dir
	// unless the plugin lives in a subdirectory of its module.
	pkgDir     string
	env        []string
	buildFlags []string
	// skipFiles are generated sources that must not be rewritten.
	skipFiles []string
	// target is the single source file the build compiles when the plugin
	// src is a file; it is empty when the build compiles the package in dir.
	target string
}

// migratedSource is the content and mode of a file before it was rewritten.
type migratedSource struct {
	content []byte
	mode    fs.FileMode
}

// migrationResult reports what migrateExtractorCalls did.
type migrationResult struct {
	// changes describes each rewritten call site.
	changes []string
	// manual describes each call site that has to be migrated by hand.
	manual []string
	// originals holds every file that was written, or was about to be when
	// the write failed, so that the caller can put it back.
	originals map[string]migratedSource
}

const (
	loadMode = packages.NeedName |
		packages.NeedFiles |
		packages.NeedCompiledGoFiles |
		packages.NeedImports |
		packages.NeedSyntax |
		packages.NeedTypes |
		packages.NeedTypesInfo

	stdContextPath = "context"
	errName        = "err"
)

// extractorMethods are the query-go extractor methods whose calls are migrated.
var extractorMethods = []string{"ExtractByKey", "ExtractByIndex"}

// migrateExtractorCalls type-checks the plugin sources in m.dir and rewrites
// the calls to scenarigo's extractor methods that still have the query-go v1
// shape. It reports what it changed, what it could not change, and the content
// of every file it wrote.
func migrateExtractorCalls(ctx context.Context, m *migration) (*migrationResult, error) {
	res := &migrationResult{originals: map[string]migratedSource{}}
	dir, err := filepath.Abs(m.dir)
	if err != nil {
		return res, err
	}
	pkgDir := dir
	if m.pkgDir != "" {
		if pkgDir, err = filepath.Abs(m.pkgDir); err != nil {
			return res, err
		}
	}
	target := ""
	if m.target != "" {
		target = realPath(m.target)
	}
	pattern, err := buildPattern(dir, pkgDir, m.target)
	if err != nil {
		return res, err
	}
	pkgs, err := loadPluginPackages(ctx, m, dir, pattern)
	if err != nil {
		return res, err
	}
	if len(pkgs) == 0 {
		return res, fmt.Errorf("no package found for %s", pattern)
	}
	if err := checkLoaded(pkgs); err != nil {
		return res, err
	}
	realDir := realPath(dir)
	realPkgDir := realPath(pkgDir)

	var edits []*fileEdits
	for _, pkg := range pkgs {
		root := realPath(pkgDirOf(pkg)) == realPkgDir
		for i, file := range pkg.Syntax {
			if i >= len(pkg.CompiledGoFiles) {
				break
			}
			path := pkg.CompiledGoFiles[i]
			if root && target != "" && realPath(path) != target {
				// A single-file build compiles only that file of its package.
				continue
			}
			if !migratable(path, dir, realDir, m.skipFiles) {
				continue
			}
			fe, err := planFileEdits(pkg, file, path, dir, res)
			if err != nil {
				return res, err
			}
			if fe != nil {
				edits = append(edits, fe)
			}
		}
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].path < edits[j].path })
	for _, fe := range edits {
		if err := fe.apply(res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// loadPluginPackages loads the packages the failed build compiles: the one the
// build names and, transitively, the ones inside dir that it imports.
//
// The set comes from "go list -deps" rather than from the "./..." pattern,
// which is a pattern matcher and not a build graph: it skips directories named
// testdata and those starting with "." or "_", although a package may import
// them and the build then compiles them. Everything outside dir - the standard
// library, other modules - is dropped, and so is a vendor tree inside dir,
// which holds other modules' sources too. Only the plugin's own sources are
// ever candidates.
func loadPluginPackages(ctx context.Context, m *migration, dir, pattern string) ([]*packages.Package, error) {
	deps, err := buildDeps(ctx, m, dir, pattern)
	if err != nil {
		return nil, err
	}
	cfg := &packages.Config{
		Context:    ctx,
		Mode:       loadMode,
		Dir:        dir,
		Env:        m.loadEnv(),
		BuildFlags: m.buildFlags,
	}
	// go list takes the file path; packages.Load needs the file= query to mix
	// it with the import paths of the dependencies.
	root := pattern
	if strings.HasSuffix(root, ".go") {
		root = "file=" + root
	}
	pkgs, err := packages.Load(cfg, append([]string{root}, deps...)...)
	if err != nil {
		return nil, fmt.Errorf("failed to load the plugin packages: %w", err)
	}
	return pkgs, nil
}

// goCommand is the go command to run, defaulting to the one on PATH.
func (m *migration) goCommand() string {
	if m.goCmd != "" {
		return m.goCmd
	}
	return "go"
}

// env returns the environment for the go command and for packages.Load, with
// the directory of the selected go command first on PATH so that the loader,
// which runs "go" itself, uses the same toolchain as the build.
func (m *migration) loadEnv() []string {
	dir := filepath.Dir(m.goCommand())
	if dir == "" || dir == "." {
		return m.env
	}
	path := os.Getenv("PATH")
	for _, kv := range m.env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = v
		}
	}
	return append(slices.Clone(m.env), "PATH="+dir+string(os.PathListSeparator)+path)
}

// buildDeps returns the import paths of the packages inside dir that the build
// of pattern compiles. The package the pattern itself names is kept - only the
// synthesized name a single-file build gets is dropped, since it is not an
// importable path - and packages.Load deduplicates it against the pattern.
//
// Vendored packages are dropped too: a vendor tree sits inside dir but holds
// other modules' sources, which the migration must not rewrite.
func buildDeps(ctx context.Context, m *migration, dir, pattern string) ([]string, error) {
	args := append([]string{"list", "-deps", "-f", "{{.ImportPath}}\t{{.Dir}}"}, m.buildFlags...)
	out, err := executeWithEnvs(ctx, m.env, dir, m.goCommand(), append(args, pattern)...)
	if err != nil {
		return nil, fmt.Errorf("failed to list the plugin dependencies: %w", err)
	}
	realDir := realPath(dir)
	var deps []string
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		path, pkgDir, ok := strings.Cut(line, "\t")
		if !ok || pkgDir == "" || path == "command-line-arguments" {
			continue
		}
		if within(realPath(pkgDir), realDir) && !vendored(realPath(pkgDir), realDir) {
			deps = append(deps, path)
		}
	}
	return deps, nil
}

// vendored reports whether path sits under a vendor directory inside dir.
func vendored(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return slices.Contains(strings.Split(rel, string(filepath.Separator)), "vendor")
}

// checkLoaded reports the first package whose sources were not parsed and
// type-checked. Other errors are expected: the build just failed, and go list
// reports that failure too while it tries to export the package.
func checkLoaded(pkgs []*packages.Package) error {
	for _, pkg := range pkgs {
		if pkg.Types == nil || pkg.TypesInfo == nil || len(pkg.Syntax) != len(pkg.CompiledGoFiles) {
			if len(pkg.Errors) > 0 {
				return fmt.Errorf("failed to load the plugin packages: %w", pkg.Errors[0])
			}
			return fmt.Errorf("failed to load the plugin package %s", pkg.PkgPath)
		}
	}
	return nil
}

// buildPattern is the argument that names what the build compiles, as the go
// command sees it: the single source file, or the package directory relative to
// the module root.
func buildPattern(dir, pkgDir, target string) (string, error) {
	if target != "" {
		return filepath.Abs(target)
	}
	rel, err := filepath.Rel(dir, pkgDir)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return ".", nil
	}
	return "." + string(filepath.Separator) + rel, nil
}

func pkgDirOf(pkg *packages.Package) string {
	if pkg.Dir != "" {
		return pkg.Dir
	}
	if len(pkg.CompiledGoFiles) > 0 {
		return filepath.Dir(pkg.CompiledGoFiles[0])
	}
	return ""
}

// realPath resolves the symbolic links in path, or returns path as is when it
// cannot.
func realPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// migratable reports whether the source file at path may be rewritten: it is
// a non-test file under dir that was not generated for the build, and it does
// not lead out of dir through a symbolic link.
func migratable(path, dir, realDir string, skipFiles []string) bool {
	if slices.Contains(skipFiles, path) || strings.HasSuffix(path, "_test.go") {
		return false
	}
	if !within(path, dir) {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	return err == nil && within(resolved, realDir)
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// edit replaces the bytes in [start, end) of the original file with text. An
// edit with start == end inserts.
type edit struct {
	start, end int
	text       string
}

type fileEdits struct {
	path  string
	rel   string
	src   []byte
	mode  fs.FileMode
	edits []edit
}

func (fe *fileEdits) apply(res *migrationResult) error {
	sort.Slice(fe.edits, func(i, j int) bool {
		if fe.edits[i].start != fe.edits[j].start {
			return fe.edits[i].start > fe.edits[j].start
		}
		return fe.edits[i].end > fe.edits[j].end
	})
	out := slices.Clone(fe.src)
	prev := len(out)
	for _, e := range fe.edits {
		if e.end > prev || e.start > e.end || e.start < 0 {
			return fmt.Errorf("failed to rewrite %s: overlapping edits", fe.rel)
		}
		out = slices.Concat(out[:e.start], []byte(e.text), out[e.end:])
		prev = e.start
	}
	// Remember the original before touching the file, so that a write that
	// fails halfway can still be undone.
	res.originals[fe.path] = migratedSource{content: fe.src, mode: fe.mode}
	if err := os.WriteFile(fe.path, out, fe.mode); err != nil {
		return fmt.Errorf("failed to write %s: %w", fe.rel, err)
	}
	return nil
}

// planner collects the edits for one file.
type planner struct {
	pkg     *packages.Package
	file    *ast.File
	fe      *fileEdits
	res     *migrationResult
	parents map[ast.Node]ast.Node
	// background is set when a call needs context.Background(), so that the
	// standard context package must be importable from this file.
	background bool
	// renamedIn records the scopes in which a variable was renamed to err,
	// so that a second variable in the same scope keeps its own name instead
	// of sharing err with the first.
	renamedIn map[*types.Scope]bool
}

func planFileEdits(pkg *packages.Package, file *ast.File, path, dir string, res *migrationResult) (*fileEdits, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		rel = path
	}
	p := &planner{
		pkg:       pkg,
		file:      file,
		fe:        &fileEdits{path: path, rel: rel, src: src, mode: info.Mode()},
		res:       res,
		parents:   parentMap(file),
		renamedIn: map[*types.Scope]bool{},
	}

	var calls []*ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && p.isExtractorCall(call) {
			calls = append(calls, call)
		}
		return true
	})
	if len(calls) == 0 {
		return nil, nil //nolint:nilnil // nil means the file needs no edits
	}
	for _, call := range calls {
		sel := call.Fun.(*ast.SelectorExpr) //nolint:forcetypeassert // checked by isExtractorCall
		if p.text(sel.Sel.Pos(), sel.Sel.End()) != sel.Sel.Name {
			return nil, fmt.Errorf("%s: the source changed while it was being analyzed", rel)
		}
	}
	ctxName, importEdit := p.stdContextName()
	for _, call := range calls {
		p.planCall(call, ctxName)
	}
	if len(p.fe.edits) == 0 {
		return nil, nil //nolint:nilnil // nil means the file needs no edits
	}
	if p.background && importEdit != nil {
		p.fe.edits = append(p.fe.edits, *importEdit)
	}
	return p.fe, nil
}

// isExtractorCall reports whether call invokes an extractor method of a
// scenarigo-owned type that has the query-go v2 signature.
func (p *planner) isExtractorCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !slices.Contains(extractorMethods, sel.Sel.Name) {
		return false
	}
	fn, ok := p.pkg.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	named := namedType(sig.Recv().Type())
	if named == nil || named.Obj().Pkg() == nil || !inScenarigoModule(named.Obj().Pkg().Path()) {
		return false
	}
	return isV2ExtractorSignature(sig)
}

func namedType(t types.Type) *types.Named {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, _ := t.(*types.Named)
	return named
}

func inScenarigoModule(pkgPath string) bool {
	for _, mod := range []string{newScenarigoModPath, oldScenarigoModPath} {
		if pkgPath == mod || strings.HasPrefix(pkgPath, mod+"/") {
			return true
		}
	}
	return false
}

// isV2ExtractorSignature reports whether sig is
//
//	func(context.Context, string|int) (any, error)
func isV2ExtractorSignature(sig *types.Signature) bool {
	params, results := sig.Params(), sig.Results()
	if params.Len() != 2 || results.Len() != 2 {
		return false
	}
	if !isStdContext(params.At(0).Type()) {
		return false
	}
	if b, ok := types.Unalias(params.At(1).Type()).(*types.Basic); !ok || (b.Kind() != types.String && b.Kind() != types.Int) {
		return false
	}
	if iface, ok := types.Unalias(results.At(0).Type()).(*types.Interface); !ok || iface.NumMethods() != 0 {
		return false
	}
	return types.Identical(results.At(1).Type(), types.Universe.Lookup("error").Type())
}

func isStdContext(t types.Type) bool {
	named := namedType(t)
	return named != nil && named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == stdContextPath && named.Obj().Name() == "Context"
}

func isScenarigoContext(t types.Type) bool {
	if _, ok := types.Unalias(t).(*types.Pointer); !ok {
		return false
	}
	named := namedType(t)
	return named != nil && named.Obj().Pkg() != nil &&
		inScenarigoModule(named.Obj().Pkg().Path()) &&
		strings.HasSuffix(named.Obj().Pkg().Path(), "/context") &&
		named.Obj().Name() == "Context"
}

// binding describes how the two results of a call are bound.
type binding struct {
	kind   bindingKind
	second *ast.Ident // the identifier bound to the second result, if any
	reason string     // why the call cannot be migrated, for bindingManual
}

type bindingKind int

const (
	// bindingNone: the results are discarded or the second one is "_".
	bindingNone bindingKind = iota
	// bindingDefine: the second result defines a new variable.
	bindingDefine
	// bindingManual: the call must be migrated by hand.
	bindingManual
)

func (p *planner) bindingOf(call *ast.CallExpr) binding {
	switch parent := p.parents[call].(type) {
	case *ast.ExprStmt:
		return binding{kind: bindingNone}
	case *ast.AssignStmt:
		if len(parent.Rhs) != 1 || parent.Rhs[0] != call || len(parent.Lhs) != 2 {
			return binding{kind: bindingManual, reason: "its results are not bound by a two-value assignment"}
		}
		return p.bindingOfIdent(parent.Lhs[1], parent.Tok == token.DEFINE)
	case *ast.ValueSpec:
		if len(parent.Values) != 1 || parent.Values[0] != call || len(parent.Names) != 2 {
			return binding{kind: bindingManual, reason: "its results are not bound by a two-value declaration"}
		}
		return p.bindingOfIdent(parent.Names[1], true)
	}
	return binding{kind: bindingManual, reason: "its results are not bound by a two-value assignment"}
}

func (p *planner) bindingOfIdent(expr ast.Expr, define bool) binding {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return binding{kind: bindingManual, reason: "its second result is assigned to an existing variable"}
	}
	if id.Name == "_" {
		return binding{kind: bindingNone}
	}
	if !define || p.pkg.TypesInfo.Defs[id] == nil {
		return binding{kind: bindingManual, reason: "its second result is assigned to an existing variable"}
	}
	return binding{kind: bindingDefine, second: id}
}

func (p *planner) planCall(call *ast.CallExpr, ctxName string) {
	sel := call.Fun.(*ast.SelectorExpr) //nolint:forcetypeassert // checked by isExtractorCall
	pos := p.pkg.Fset.Position(sel.Sel.Pos())
	site := fmt.Sprintf("%s:%d:%d", p.fe.rel, pos.Line, pos.Column)

	var insertCtx bool
	switch len(call.Args) {
	case 1:
		// The v1 shape: the arity alone proves the call has to change.
		insertCtx = true
	case 2:
		// Already the v2 arity. Only the bool-shaped v1 form of a
		// context-taking extractor is left, which shows as a type error at
		// a use of the second result.
	default:
		return
	}

	b := p.bindingOf(call)
	var (
		conv  *conversion
		spans []ast.Node
	)
	if b.kind == bindingDefine {
		obj, _ := p.pkg.TypesInfo.Defs[b.second].(*types.Var)
		if obj == nil {
			return
		}
		uses := p.usesOf(obj)
		spans = p.spansOf(uses)
		if obj.Parent() == p.pkg.Types.Scope() {
			b = binding{kind: bindingManual, reason: "its second result is a package-level variable"}
		} else if conv = p.conversionOf(b.second, obj, uses); conv == nil {
			b = binding{kind: bindingManual, reason: "its second result is assigned to later"}
		}
	}
	if !insertCtx && !p.hardTypeErrorAt(spans) && !p.boolMismatchAt(call) {
		// The v2 arity and no sign of the v1 shape: correct v2 code.
		return
	}
	var (
		ctxExpr    string
		background bool
	)
	if insertCtx {
		ctxExpr, background = p.contextExprFor(sel, ctxName)
		if ctxExpr == "" && b.kind != bindingManual {
			b = binding{kind: bindingManual, reason: "the file has no free name to import the context package as"}
		}
	}
	if b.kind == bindingManual {
		p.res.manual = append(p.res.manual, fmt.Sprintf(
			"%s: %s cannot be migrated automatically because %s: rewrite it by hand as v, err := X.%s(ctx, key) and test err (errors.Is(err, query.ErrNotFound) reports absence)",
			site, sel.Sel.Name, b.reason, sel.Sel.Name))
		return
	}

	var change strings.Builder
	fmt.Fprintf(&change, "%s: %s(%s)", site, sel.Sel.Name, p.text(call.Args[0].Pos(), call.Args[len(call.Args)-1].End()))
	if insertCtx {
		if background {
			p.background = true
		}
		p.fe.edits = append(p.fe.edits, edit{
			start: p.offset(call.Args[0].Pos()),
			end:   p.offset(call.Args[0].Pos()),
			text:  ctxExpr + ", ",
		})
		fmt.Fprintf(&change, " => %s(%s, %s)", sel.Sel.Name, ctxExpr, p.text(call.Args[0].Pos(), call.Args[len(call.Args)-1].End()))
	}
	if conv != nil {
		p.fe.edits = append(p.fe.edits, conv.edits...)
		if conv.renamed {
			p.renamedIn[conv.scope] = true
			fmt.Fprintf(&change, ", %s => %s", b.second.Name, errName)
		} else {
			fmt.Fprintf(&change, ", %s is now an error", b.second.Name)
		}
	}
	p.res.changes = append(p.res.changes, change.String())
}

// conversion is the set of edits that turns a bool variable into an error
// variable: the definition may be renamed, and every use becomes a nil test.
type conversion struct {
	edits   []edit
	renamed bool
	// scope is the scope the variable is renamed in; it is marked as taken
	// only when the conversion is applied.
	scope *types.Scope
}

// usesOf returns the identifiers in the file that refer to obj.
func (p *planner) usesOf(obj *types.Var) []*ast.Ident {
	var uses []*ast.Ident
	ast.Inspect(p.file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && p.pkg.TypesInfo.Uses[id] == obj {
			uses = append(uses, id)
		}
		return true
	})
	return uses
}

// spansOf returns the expressions the type checker rejects when the variable
// the uses refer to is not a bool: each use, the "!use" negating it, or the
// statement writing to it.
func (p *planner) spansOf(uses []*ast.Ident) []ast.Node {
	spans := make([]ast.Node, 0, len(uses))
	for _, id := range uses {
		var span ast.Node = id
		if u, ok := p.parents[id].(*ast.UnaryExpr); ok && u.Op == token.NOT {
			span = u
		} else if p.isWriteTarget(id) {
			span = p.parents[id]
		}
		spans = append(spans, span)
	}
	return spans
}

func (p *planner) conversionOf(def *ast.Ident, obj *types.Var, uses []*ast.Ident) *conversion {
	if slices.ContainsFunc(uses, p.isWriteTarget) {
		return nil
	}

	name := obj.Name()
	c := &conversion{}
	if name != errName && p.canRename(def, uses, errName) {
		name = errName
		c.renamed = true
		c.edits = append(c.edits, edit{start: p.offset(def.Pos()), end: p.offset(def.End()), text: name})
		c.scope = p.pkg.Types.Scope().Innermost(def.Pos())
	}
	for _, id := range uses {
		start, end := p.offset(id.Pos()), p.offset(id.End())
		parent := p.parents[id]
		text := name + " == nil"
		if u, ok := parent.(*ast.UnaryExpr); ok && u.Op == token.NOT {
			start, end = p.offset(u.Pos()), p.offset(u.End())
			text = name + " != nil"
			parent = p.parents[u]
		}
		if _, ok := parent.(*ast.BinaryExpr); ok {
			text = "(" + text + ")"
		}
		c.edits = append(c.edits, edit{start: start, end: end, text: text})
	}
	return c
}

// isWriteTarget reports whether id is assigned to, taken the address of, or
// incremented: a bool variable used that way cannot become an error variable.
func (p *planner) isWriteTarget(id *ast.Ident) bool {
	switch parent := p.parents[id].(type) {
	case *ast.AssignStmt:
		return slices.Contains(parent.Lhs, ast.Expr(id))
	case *ast.UnaryExpr:
		return parent.Op == token.AND
	case *ast.IncDecStmt:
		return true
	}
	return false
}

// canRename reports whether the variable can be called name without changing
// what name refers to anywhere the variable is defined or used.
func (p *planner) canRename(def *ast.Ident, uses []*ast.Ident, name string) bool {
	scope := p.pkg.Types.Scope()
	for _, id := range append([]*ast.Ident{def}, uses...) {
		inner := scope.Innermost(id.Pos())
		if inner == nil {
			return false
		}
		if _, obj := inner.LookupParent(name, id.Pos()); obj != nil {
			return false
		}
	}
	// A later declaration in the defining scope would clash with the
	// renamed variable, and so would an earlier rename in an enclosing
	// scope: the two variables must not share one name.
	inner := scope.Innermost(def.Pos())
	if inner == nil || inner.Lookup(name) != nil {
		return false
	}
	for s := inner; s != nil; s = s.Parent() {
		if p.renamedIn[s] {
			return false
		}
	}
	return true
}

// hardTypeErrorAt reports whether the type checker rejected one of the given
// expressions. The check is limited to the expressions themselves so that an
// error elsewhere in the same statement, such as a v1 call sharing a
// condition with an already migrated one, is not taken as evidence. Soft
// errors (an unused variable) are not evidence either.
func (p *planner) hardTypeErrorAt(spans []ast.Node) bool {
	for _, e := range p.pkg.TypeErrors {
		if e.Soft {
			continue
		}
		for _, s := range spans {
			if s.Pos() <= e.Pos && e.Pos < s.End() {
				return true
			}
		}
	}
	return false
}

// boolMismatchAt reports whether the type checker rejected the call's results
// where a bool was expected, as when a call that already has the v2 arity is
// returned or assigned as is into a bool.
func (p *planner) boolMismatchAt(call *ast.CallExpr) bool {
	for _, e := range p.pkg.TypeErrors {
		if !e.Soft && e.Pos == call.Pos() && strings.Contains(e.Msg, "as bool value") {
			return true
		}
	}
	return false
}

// contextExprFor returns the source text of the context to pass to the
// extractor. When the receiver is reached from a scenarigo context, its
// request context carries the deadline and cancellation of the running step;
// otherwise context.Background() is the only context there is, and the bool
// reports that fallback. The text is empty when the fallback is needed but
// the file has no name for the context package.
func (p *planner) contextExprFor(sel *ast.SelectorExpr, ctxName string) (string, bool) {
	e := sel.X
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
			continue
		case *ast.CallExpr:
			e = x.Fun
			continue
		case *ast.SelectorExpr:
			if isScenarigoContext(p.pkg.TypesInfo.TypeOf(x)) && sideEffectFree(x) {
				return p.text(x.Pos(), x.End()) + ".RequestContext()", false
			}
			e = x.X
			continue
		case *ast.Ident:
			if isScenarigoContext(p.pkg.TypesInfo.TypeOf(x)) {
				return x.Name + ".RequestContext()", false
			}
		}
		break
	}
	if ctxName == "" {
		return "", true
	}
	return ctxName + ".Background()", true
}

// sideEffectFree reports whether evaluating e twice is harmless: e is an
// identifier or a chain of field selections on one.
func sideEffectFree(e ast.Expr) bool {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return true
		case *ast.SelectorExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return false
		}
	}
}

// stdContextName returns the name the standard context package has in this
// file and, when it is not imported yet, the edit that imports it. The name
// is empty when the package is not imported and no name is free for it.
func (p *planner) stdContextName() (string, *edit) {
	for _, imp := range p.file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != stdContextPath {
			continue
		}
		if imp.Name == nil {
			return stdContextPath, nil
		}
		if imp.Name.Name != "_" && imp.Name.Name != "." {
			return imp.Name.Name, nil
		}
	}
	for i, name := range []string{stdContextPath, "gocontext", "gocontext2", "gocontext3"} {
		if !p.nameIsFree(name) {
			continue
		}
		spec := fmt.Sprintf("\n\nimport %q", stdContextPath)
		if i > 0 {
			spec = fmt.Sprintf("\n\nimport %s %q", name, stdContextPath)
		}
		off := p.offset(p.file.Name.End())
		return name, &edit{start: off, end: off, text: spec}
	}
	return "", nil
}

// nameIsFree reports whether name is bound nowhere in the file or package, so
// an import can take it.
func (p *planner) nameIsFree(name string) bool {
	for _, imp := range p.file.Imports {
		if imp.Name != nil {
			if imp.Name.Name == name {
				return false
			}
			continue
		}
		if pn, ok := p.pkg.TypesInfo.Implicits[imp].(*types.PkgName); ok && pn.Name() == name {
			return false
		}
		if path.Base(strings.Trim(imp.Path.Value, `"`)) == name {
			return false
		}
	}
	if p.pkg.Types.Scope().Lookup(name) != nil {
		return false
	}
	free := true
	ast.Inspect(p.file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			if p.pkg.TypesInfo.Defs[id] != nil || p.pkg.TypesInfo.Uses[id] != nil {
				free = false
			}
		}
		return free
	})
	return free
}

func (p *planner) offset(pos token.Pos) int {
	return p.pkg.Fset.Position(pos).Offset
}

func (p *planner) text(from, to token.Pos) string {
	// The positions come from the type-checked package, the bytes from the file
	// read afterwards. If the file changed in between the positions can point
	// past its end, so report no text rather than panicking here: the caller
	// compares the result and reports the change.
	lo, hi := p.offset(from), p.offset(to)
	if lo < 0 || hi > len(p.fe.src) || lo > hi {
		return ""
	}
	return string(bytes.TrimSpace(p.fe.src[lo:hi]))
}

func parentMap(file *ast.File) map[ast.Node]ast.Node {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return parents
}
