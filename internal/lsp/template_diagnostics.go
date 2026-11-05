package lsp

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml/ast"

	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
	tmplast "github.com/scenarigo/scenarigo/template/ast"
	tmplparser "github.com/scenarigo/scenarigo/template/parser"
	tmpltoken "github.com/scenarigo/scenarigo/template/token"
)

// Template diagnostics.
//
// Template expressions ({{ }}) are parsed with scenarigo's own template
// parser, so a syntax error is reported exactly where "scenarigo run" would
// fail. Names are then checked against what is defined at the point where
// scenarigo evaluates the expression: the vars of a scenario are evaluated
// before they are added, a step's if and vars before the step's vars, and a
// step's bind and id are visible to the following steps only. Values that
// only exist at run time get a lower severity: a plugin's setup function can
// add variables with ctx.WithVars, so an undefined vars.x or secrets.x is
// only information.

// templateContextRoots are the names the template context resolves
// (context/extract.go).
var templateContextRoots = []string{"ctx", nsPlugins, nsVars, nsSecrets, nsSteps, "request", "response", "env", "assert"}

// templateBuiltins are the functions the template package resolves before
// the context: size, type, and the type conversions of template/val.
var templateBuiltins = []string{"size", "type", "any", "int", "uint", "float", "bool", "string", "bytes", "time", "duration"}

// templateActualValue is the value under test inside expect ({{$ != ""}}).
const templateActualValue = "$"

// maxTemplateSize bounds the strings given to the template parser. The parser
// and the checker recurse once per level of nesting, and running out of stack
// is fatal to the process, so recover would not help: 2 MB of "(" ends the
// server.
const maxTemplateSize = 1 << 20

// Fields whose string values scenarigo evaluates as templates.
var stepEvaluatedFields = []string{"request", "expect", "ref"}

// templateDefinitions holds the names an expression can refer to.
type templateDefinitions struct {
	vars, secrets, steps, plugins map[string]bool
}

func newTemplateDefinitions() templateDefinitions {
	return templateDefinitions{vars: map[string]bool{}, secrets: map[string]bool{}, steps: map[string]bool{}, plugins: map[string]bool{}}
}

func (d templateDefinitions) clone() templateDefinitions {
	return templateDefinitions{vars: maps.Clone(d.vars), secrets: maps.Clone(d.secrets), steps: maps.Clone(d.steps), plugins: maps.Clone(d.plugins)}
}

func (d templateDefinitions) add(set map[string]bool, names []string) {
	for _, n := range names {
		set[n] = true
	}
}

// addNew adds names to a set and returns those it did not hold yet.
func (d templateDefinitions) addNew(set map[string]bool, names []string) []string {
	var added []string
	for _, n := range names {
		if !set[n] {
			set[n] = true
			added = append(added, n)
		}
	}
	return added
}

func removeNames(set map[string]bool, names []string) {
	for _, n := range names {
		delete(set, n)
	}
}

func (d templateDefinitions) merge(o templateDefinitions) {
	maps.Copy(d.vars, o.vars)
	maps.Copy(d.secrets, o.secrets)
	maps.Copy(d.steps, o.steps)
	maps.Copy(d.plugins, o.plugins)
}

func (s *Server) templateDiagnostics(doc *document) []Diagnostic {
	if doc.Parsed == nil || doc.Parsed.File == nil || doc.Parsed.Text != doc.Text {
		// The positions of a cached AST do not match the edited text.
		return nil
	}
	var diags []Diagnostic
	docPath := uriToPath(doc.URI)
	c := &templateChecker{text: doc.Text, lines: newLineIndex(doc.Text), diags: &diags, walked: map[ast.Node]bool{}}
	var includers *includerScope
	c.includers = func() includerScope {
		if includers == nil {
			names, complete := s.includerNames(docPath)
			includers = &includerScope{defs: names, complete: complete}
		}
		return *includers
	}
	config := s.configDefinitions()
	// What a ytt annotation fills in is not in this text: names, ids and includes.
	c.unresolved = strings.Contains(doc.Text, "#@")
	unresolved := c.unresolved
	for v, d := range yamlDocuments(doc.Parsed.File) {
		c.view = v
		body := v.entries(d.Body)
		if body == nil {
			continue
		}
		switch v.stringValue(v.entryValue(body, "schemaVersion")) {
		case "config/v1":
			c.defs = config
			c.walkYAML(v.entryValue(body, nsVars))
			c.walkYAML(v.entryValue(body, nsSecrets))
			// The protocol options are evaluated for each request, in the
			// context of the step: names are not known here.
			c.unresolved = true
			c.walkYAML(v.entryValue(body, "protocols"))
			c.unresolved = unresolved
		case "ytt/v1":
			// File names only.
		default:
			s.checkScenario(c, body, config, docPath)
		}
	}
	return diags
}

// checkScenario checks a scenario document in the order scenarigo evaluates
// it (scenario.go and step.go).
func (s *Server) checkScenario(c *templateChecker, body []*ast.MappingValueNode, config templateDefinitions, docPath string) {
	v := c.view
	unresolved := c.unresolved
	defs := config.clone()
	defs.add(defs.plugins, v.keys(v.entryValue(body, nsPlugins))) // opened first
	c.defs = defs
	c.walkShared(v.entryValue(body, nsVars))
	defs.add(defs.vars, v.keys(v.entryValue(body, nsVars)))
	c.walkShared(v.entryValue(body, nsSecrets))
	defs.add(defs.secrets, v.keys(v.entryValue(body, nsSecrets)))

	for _, step := range v.sequence(v.entryValue(body, nsSteps)) {
		entries := v.entries(step)
		// The vars and secrets of a step are added for the step only and
		// removed after it: copying the definitions for each step made
		// scenarios of many steps quadratic.
		c.defs = defs
		c.walkShared(v.entryValue(entries, "if"))
		c.walkShared(v.entryValue(entries, nsVars))
		stepVars := defs.addNew(defs.vars, v.keys(v.entryValue(entries, nsVars)))
		c.walkShared(v.entryValue(entries, nsSecrets))
		stepSecrets := defs.addNew(defs.secrets, v.keys(v.entryValue(entries, nsSecrets)))
		include := v.stringValue(v.entryValue(entries, "include"))
		if include == "" { // the runner returns before the other fields
			for _, n := range v.entryValues(entries, stepEvaluatedFields) {
				c.walkShared(n)
			}
		}

		// The bind of an include step reads the final context of the
		// included scenario: its steps instead of these ones, and its vars
		// on top of these.
		bind := v.entryValue(entries, "bind")
		if include != "" {
			included, ok := templateDefinitions{}, false
			if docPath != "" {
				included, ok = s.includedNames(c, includedPath(docPath, include))
			}
			c.defs.steps = included.steps // c.defs is a copy of defs, sharing its sets
			c.bound = included
			c.unresolved = unresolved || !ok
			c.walkShared(bind)
			c.unresolved = unresolved
			c.defs, c.bound = defs, templateDefinitions{}
		} else {
			c.walkShared(bind)
		}
		removeNames(defs.vars, stepVars)
		removeNames(defs.secrets, stepSecrets)

		bindEntries := v.entries(bind)
		defs.add(defs.vars, v.keys(v.entryValue(bindEntries, nsVars)))
		defs.add(defs.secrets, v.keys(v.entryValue(bindEntries, nsSecrets)))
		if id := v.stringValue(v.entryValue(entries, "id")); id != "" {
			defs.steps[id] = true
		}
	}
}

// includedNames returns the names of the scenario at path, reading each
// file once per checker: a scenario that includes a file many times would
// otherwise read and parse it for each include step. The names are only
// read, never changed.
func (s *Server) includedNames(c *templateChecker, path string) (templateDefinitions, bool) {
	if r, ok := c.included[path]; ok {
		return r.defs, r.ok
	}
	defs, ok := s.scenarioNames(path, false)
	if c.included == nil {
		c.included = map[string]includedScenario{}
	}
	c.included[path] = includedScenario{defs: defs, ok: ok}
	return defs, ok
}

type includedScenario struct {
	defs templateDefinitions
	ok   bool
}

// configDefinitions returns the names scenarigo.yaml defines.
func (s *Server) configDefinitions() templateDefinitions {
	defs := newTemplateDefinitions()
	configText, _, ok := s.readConfigText()
	if !ok {
		return defs
	}
	parsed := yamlutil.Parse(configText)
	if parsed == nil || parsed.File == nil {
		// Broken while it is being edited: the lines are the best guess.
		defs.add(defs.vars, extractBlockKeys(configText, nsVars))
		defs.add(defs.secrets, extractBlockKeys(configText, nsSecrets))
		return defs
	}
	for v, d := range yamlDocuments(parsed.File) {
		body := v.entries(d.Body)
		defs.add(defs.vars, v.keys(v.entryValue(body, nsVars)))
		defs.add(defs.secrets, v.keys(v.entryValue(body, nsSecrets)))
	}
	return defs
}

// templateChecker reports the diagnostics of the template strings in a YAML
// subtree.
type templateChecker struct {
	text  string
	lines lineIndex
	view  *yamlView
	defs  templateDefinitions
	diags *[]Diagnostic
	// includers returns the names the scenarios that include this file
	// define; it is called only for a name that is not defined otherwise.
	includers func() includerScope
	// unresolved is set where the names are not known, so that undefined
	// names are not reported: they come from a file that cannot be read, from
	// the step that evaluates a protocol option, or from ytt.
	unresolved bool
	// walked holds the nodes aliases and merge keys can reach that are checked
	// already.
	walked map[ast.Node]bool
	// absorbing is set inside the index of a path under ?? or defined().
	absorbing bool
	// branches counts the operands around the expression that may not run.
	branches int
	// bound holds, in the bind of an include step, the names of the
	// included scenario, seen on top of defs without adding them to defs for
	// each include step.
	bound templateDefinitions
	// included holds the names of the scenarios included so far, by path.
	included map[string]includedScenario
}

// includerScope holds the names of the scenarios that include a file.
// complete is false when a scenario that may include the file could not be
// read, or a file has a path but no workspace: any name may come from what was
// not read. A document without a path has no includers.
type includerScope struct {
	defs     templateDefinitions
	complete bool
}

// walkYAML checks the strings of a YAML subtree. Scenarigo evaluates what
// aliases and merge keys bring, where they are used.
func (c *templateChecker) walkYAML(node ast.Node) {
	switch n := node.(type) {
	case *ast.AnchorNode:
		c.walkShared(n.Value)
	case *ast.AliasNode:
		c.walkShared(c.view.unwrap(n))
	case *ast.TagNode:
		c.walkYAML(n.Value)
	case *ast.MappingNode:
		if slices.ContainsFunc(n.Values, isMergeEntry) {
			c.walkMerged(n)
			return
		}
		for _, mv := range n.Values {
			c.walkYAML(mv)
		}
	case *ast.MappingValueNode:
		if isMergeEntry(n) {
			c.walkMerged(n)
			return
		}
		c.walkYAML(n.Key) // left arrow functions and templated keys
		c.walkYAML(n.Value)
	case *ast.SequenceNode:
		for _, v := range n.Values {
			c.walkYAML(v)
		}
	case *ast.StringNode:
		if tok := n.GetToken(); tok != nil && strings.Contains(n.Value, "{{") {
			c.checkString(n.Value, c.lines.offset(c.text, tok.Position.Line, tok.Position.Column))
		}
	case *ast.LiteralNode:
		// The content of a block scalar starts on the line after its header.
		if tok := n.GetToken(); tok != nil && n.Value != nil {
			c.checkString(n.Value.Value, c.lines.lineStart(tok.Position.Line))
		}
	}
}

// walkShared checks a node that aliases and merge keys can reach, once, in the
// scope of its first use. A name undefined there is a failure whatever the
// other uses do, while checking every use would report it once per use, cost
// time per use, and multiply with nested aliases: [*a, *a] of [*b, *b] and so
// on.
func (c *templateChecker) walkShared(node ast.Node) {
	if node == nil || c.walked[node] {
		return
	}
	c.walked[node] = true
	c.walkYAML(node)
}

// walkMerged checks a mapping with merge keys. Of a key written twice, it
// checks the value view.entries gives first: the only one the loader keeps in
// vars and secrets, and the last written of the two a nested mapping keeps.
func (c *templateChecker) walkMerged(node ast.Node) {
	seen := map[string]bool{}
	for _, mv := range c.view.entries(node) {
		if mv.Key != nil {
			if seen[keyName(mv.Key)] {
				continue
			}
			seen[keyName(mv.Key)] = true
		}
		c.walkShared(mv)
	}
}

func isMergeEntry(mv *ast.MappingValueNode) bool {
	_, ok := mv.Key.(*ast.MergeKeyNode)
	return ok
}

// checkString parses one YAML string value that starts at the byte offset
// start of the text.
func (c *templateChecker) checkString(value string, start int) {
	if start < 0 || len(value) > maxTemplateSize || !strings.Contains(value, "{{") {
		return
	}
	m := newValueMap(c.text, c.lines, start, value)
	node, err := tmplparser.NewParser(strings.NewReader(value)).Parse()
	if err != nil {
		// Later errors usually follow from the first one.
		col, msg := parseErrorPosition(err)
		*c.diags = append(*c.diags, Diagnostic{
			Range:    m.runeRange(col-1, col),
			Severity: DiagnosticSeverityError,
			Message:  "invalid template: " + msg,
		})
		return
	}
	expr, ok := node.(tmplast.Expr)
	if !ok {
		return
	}
	c.walkExpr(expr, m, false)
}

// parseErrorPosition returns the 1-based rune column and the message of the
// first parse error. The parser exposes them through Error() only, as
// "col N: message".
func parseErrorPosition(err error) (int, string) {
	var errs tmplparser.Errors
	if errors.As(err, &errs) && len(errs) > 0 {
		err = errs[0]
	}
	var col int
	text := err.Error()
	if _, scanErr := fmt.Sscanf(text, "col %d:", &col); scanErr == nil {
		if _, msg, ok := strings.Cut(text, ": "); ok {
			return col, msg
		}
	}
	return 0, text
}

// walkExpr checks the names used in expr. optional is set for a path that
// ?? or defined() absorbs when it is missing: at run time they do so only
// for an identifier, a selector or an index directly on their left or in
// their argument (template.go).
func (c *templateChecker) walkExpr(expr tmplast.Expr, m valueMap, optional bool) {
	switch e := expr.(type) {
	case *tmplast.Ident, *tmplast.SelectorExpr, *tmplast.IndexExpr:
		c.checkPath(expr, m, optional || c.absorbing)
	case *tmplast.CallExpr:
		c.walkExpr(e.Fun, m, false)
		for _, arg := range e.Args {
			c.requireOperand(arg, e.Lparen, m)
		}
	case *tmplast.LeftArrowExpr:
		c.requireOperandAt(e.Fun, e.Larrow, len("<-"), m)
		c.walkExpr(e.Arg, m, false) // no argument in a map key
	case *tmplast.ParameterExpr:
		c.walkExpr(e.X, m, false) // {{}} is an empty string
	case *tmplast.ParenExpr:
		c.requireOperand(e.X, e.Lparen, m)
	case *tmplast.UnaryExpr:
		c.requireOperandAt(e.X, e.OpPos, len(e.Op.String()), m)
	case *tmplast.BinaryExpr:
		width := len(e.Op.String())
		switch {
		case e.Op != tmpltoken.COALESCING:
			c.requireOperandAt(e.X, e.OpPos, width, m)
			c.requireOperandAt(e.Y, e.OpPos, width, m)
		case e.X == nil:
			c.requireOperandAt(e.X, e.OpPos, width, m)
			c.branchOperand(e.Y, e.OpPos, width, m)
		default:
			c.walkExpr(e.X, m, isPathExpr(e.X))
			c.branchOperand(e.Y, e.OpPos, width, m)
		}
	case *tmplast.ConditionalExpr:
		c.requireOperand(e.Condition, e.Question, m)
		c.branchOperand(e.X, e.Question, 1, m)
		c.branchOperand(e.Y, e.Colon, 1, m)
	case *tmplast.DefinedExpr:
		if e.Arg == nil {
			c.reportAt(m, e.DefinedPos, len("defined"), DiagnosticSeverityError, "invalid template: defined() needs an argument")
			return
		}
		if !isPathExpr(e.Arg) {
			// scenarigo fails with "invalid argument to defined()".
			c.reportAt(m, e.DefinedPos, len("defined"), DiagnosticSeverityError, "invalid template: defined() takes a name such as vars.x")
		}
		c.walkExpr(e.Arg, m, isPathExpr(e.Arg))
	}
}

// requireOperand checks an operand that the parser leaves nil when it is
// missing ({{a ==}}); scenarigo fails on it at run time.
// The diagnostic marks the one-character token at pos.
func (c *templateChecker) requireOperand(expr tmplast.Expr, pos int, m valueMap) {
	c.requireOperandAt(expr, pos, 1, m)
}

// requireOperandAt is requireOperand for a token of width runes at pos.
func (c *templateChecker) requireOperandAt(expr tmplast.Expr, pos, width int, m valueMap) {
	if expr == nil {
		c.reportAt(m, pos, width, DiagnosticSeverityError, "invalid template: missing operand")
		return
	}
	c.walkExpr(expr, m, false)
}

// branchOperand checks an operand scenarigo evaluates only when the
// expression takes that branch: {{a ?? }} runs while a is defined, and
// {{defined(a) ? a : 1}} guards its unknown name. What would fail there fails
// on that path only, so reportAt lowers it one level.
func (c *templateChecker) branchOperand(expr tmplast.Expr, pos, width int, m valueMap) {
	c.branches++
	c.requireOperandAt(expr, pos, width, m)
	c.branches--
}

func isPathExpr(expr tmplast.Expr) bool {
	switch expr.(type) {
	case *tmplast.Ident, *tmplast.SelectorExpr, *tmplast.IndexExpr:
		return true
	}
	return false
}

// templatePath is a path expression such as vars.user.name split into its
// identifiers from the root. A string index is one of them: steps["a"] reads
// the same key as steps.a. Any other index ends them: vars[0].x yields
// [vars]. indexes are the index expressions along the path, and base is the
// expression the path starts from when it is not an identifier (f().x).
type templatePath struct {
	idents  []*tmplast.Ident
	closed  bool
	indexes []*tmplast.IndexExpr
	base    tmplast.Expr
}

func pathOf(expr tmplast.Expr) templatePath {
	switch e := expr.(type) {
	case *tmplast.Ident:
		return templatePath{idents: []*tmplast.Ident{e}}
	case *tmplast.SelectorExpr:
		p := pathOf(e.X)
		if p.base == nil && !p.closed {
			p.idents = append(p.idents, e.Sel)
		}
		return p
	case *tmplast.IndexExpr:
		p := pathOf(e.X)
		if lit, ok := e.Index.(*tmplast.BasicLit); ok && lit.Kind == tmpltoken.STRING && p.base == nil && !p.closed {
			p.idents = append(p.idents, &tmplast.Ident{NamePos: lit.ValuePos + 1, Name: lit.Value})
			return p
		}
		p.closed = true
		p.indexes = append(p.indexes, e)
		return p
	}
	return templatePath{closed: true, base: expr}
}

// checkIndexes checks the indexes of a path. ?? and defined() absorb a name an
// index leaves undefined as they absorb the path itself.
func (c *templateChecker) checkIndexes(p templatePath, m valueMap, optional bool) {
	absorbing := c.absorbing
	c.absorbing = absorbing || optional
	for _, index := range p.indexes {
		c.requireOperand(index.Index, index.Lbrack, m)
	}
	c.absorbing = absorbing
}

func (c *templateChecker) checkPath(expr tmplast.Expr, m valueMap, optional bool) {
	p := pathOf(expr)
	c.checkIndexes(p, m, optional)
	if p.base != nil {
		c.walkExpr(p.base, m, false)
	}
	path := p.idents
	if len(path) == 0 {
		return
	}
	root := path[0]
	if root.Name != templateActualValue && !slices.Contains(templateContextRoots, root.Name) && !slices.Contains(templateBuiltins, root.Name) {
		if !optional {
			c.report(m, root, DiagnosticSeverityWarning, fmt.Sprintf("unknown template name %q", root.Name))
		}
		return
	}
	if len(path) < 2 {
		return
	}
	name := path[1]
	switch root.Name {
	case "assert":
		if !optional && !slices.ContainsFunc(assertCandidates(), func(a templateCandidate) bool { return a.name == name.Name }) {
			c.report(m, name, DiagnosticSeverityWarning, fmt.Sprintf("unknown assertion %q", "assert."+name.Name))
		}
	case nsSteps:
		if !optional && !c.unresolved && !c.defs.steps[name.Name] {
			c.report(m, name, DiagnosticSeverityWarning, fmt.Sprintf("no step with the id %q has run before this point of the scenario", name.Name))
		}
	case nsPlugins:
		if !optional && !c.unresolved && !c.defined(nsPlugins, name.Name) {
			c.report(m, name, DiagnosticSeverityInformation, fmt.Sprintf("plugin %q is not declared in this scenario or a scenario that includes it (a plugin setup may define it)", name.Name))
		}
	case nsVars, nsSecrets:
		if !optional && !c.unresolved && !c.defined(root.Name, name.Name) {
			c.report(m, name, DiagnosticSeverityInformation, fmt.Sprintf("%s.%s is not defined before this point (a plugin setup may define it)", root.Name, name.Name))
		}
	}
}

// defined reports whether a name is defined: in scope, by the included
// scenario in the bind of an include step, or, for all we know, by a scenario
// that includes this file.
func (c *templateChecker) defined(root, name string) bool {
	if c.defs.set(root)[name] || c.bound.set(root)[name] {
		return true
	}
	scope := c.includers()
	return !scope.complete || scope.defs.set(root)[name]
}

func (c *templateChecker) report(m valueMap, ident *tmplast.Ident, severity int, msg string) {
	c.reportAt(m, ident.NamePos, utf8.RuneCountInString(ident.Name), severity, msg)
}

// reportAt reports a diagnostic on the runes [pos, pos+length) of the value,
// where pos is a 1-based parser position.
func (c *templateChecker) reportAt(m valueMap, pos, length, severity int, msg string) {
	if c.branches > 0 {
		severity = min(severity+1, DiagnosticSeverityHint)
	}
	*c.diags = append(*c.diags, Diagnostic{
		Range:    m.runeRange(pos-1, pos-1+length),
		Severity: severity,
		Message:  msg,
	})
}

// set returns the names of a namespace.
func (d templateDefinitions) set(root string) map[string]bool {
	switch root {
	case nsVars:
		return d.vars
	case nsSecrets:
		return d.secrets
	case nsSteps:
		return d.steps
	}
	return d.plugins
}

// valueMap maps rune indexes of a decoded YAML string value to positions in
// the document.
type valueMap struct {
	lines   lineIndex
	offsets []int // byte offset of each rune of the value, plus its end; nil when unknown
	start   int   // byte offset of the scalar
	text    string
}

// newValueMap aligns the decoded value with the source text that starts at
// the byte offset start. The source differs from the value by quotes,
// escapes, indentation and folded line breaks only, so every rune of the
// value is matched to the next equal rune of the source, skipping those
// characters. Any other difference leaves the map without offsets, and
// ranges fall back to the start of the scalar.
func newValueMap(text string, lines lineIndex, start int, value string) valueMap {
	m := valueMap{lines: lines, start: start, text: text}
	offsets := make([]int, 0, utf8.RuneCountInString(value)+1)
	i := start
	for _, v := range value {
		for {
			if i >= len(text) {
				return m
			}
			r, size := utf8.DecodeRuneInString(text[i:])
			if r == v {
				offsets = append(offsets, i)
				i += size
				break
			}
			switch r {
			case ' ', '\t', '\n', '\r', '\'', '"', '\\':
				i += size
			default:
				return m
			}
		}
	}
	offsets = append(offsets, i)
	m.offsets = offsets
	return m
}

// runeRange returns the range of the runes [from, to) of the value.
func (m valueMap) runeRange(from, to int) Range {
	if len(m.offsets) == 0 {
		// Unknown layout: point at the rest of the first line of the scalar.
		start := m.lines.position(m.start)
		end := start
		end.Character += len(m.lines.text(m.text, start.Line)) - start.Character
		return Range{Start: start, End: end}
	}
	end := len(m.offsets) - 1 // index of the end of the value
	from = min(max(from, 0), end)
	to = min(max(to, from), end)
	if from == to {
		// Mark one character: the next one, or the last one past the end
		// (a missing "}}" is reported at the end of the value).
		if to < end {
			to++
		} else if from > 0 {
			from--
		}
	}
	return Range{Start: m.lines.position(m.offsets[from]), End: m.lines.position(m.offsets[to])}
}
