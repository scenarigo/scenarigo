package lsp

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml/ast"

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

// removeNames removes names from a set.
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
	var diags []Diagnostic
	s.walkTemplates(doc, &diags, nil)
	// A step that is anchored and aliased is checked twice.
	slices.SortStableFunc(diags, compareDiagnostics)
	return slices.Compact(diags)
}

// walkTemplates checks the template expressions of doc in evaluation order,
// appending the diagnostics to diags and calling onPath, when set, with every
// name path it finds.
func (s *Server) walkTemplates(doc *document, diags *[]Diagnostic, onPath func(path []*tmplast.Ident, m valueMap)) {
	if doc.Parsed == nil || doc.Parsed.File == nil || doc.Parsed.Text != doc.Text {
		// The positions of a cached AST do not match the edited text.
		return
	}
	v := newYAMLView(doc.Parsed.File)
	docPath := uriToPath(doc.URI)
	c := &templateChecker{text: doc.Text, lines: newLineIndex(doc.Text), view: v, diags: diags, onPath: onPath}
	var includers *templateDefinitions
	c.includers = func() templateDefinitions {
		if includers == nil {
			names := s.includerNames(docPath)
			includers = &names
		}
		return *includers
	}
	config := s.configDefinitions()
	for _, d := range doc.Parsed.File.Docs {
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
			c.unresolved = false
		case "ytt/v1":
			// File names only.
		default:
			s.checkScenario(c, body, config, docPath)
		}
	}
}

func compareDiagnostics(a, b Diagnostic) int {
	if a.Range.Start.Line != b.Range.Start.Line {
		return a.Range.Start.Line - b.Range.Start.Line
	}
	if a.Range.Start.Character != b.Range.Start.Character {
		return a.Range.Start.Character - b.Range.Start.Character
	}
	return strings.Compare(a.Message, b.Message)
}

// checkScenario checks a scenario document in the order scenarigo evaluates
// it (scenario.go and step.go).
func (s *Server) checkScenario(c *templateChecker, body []*ast.MappingValueNode, config templateDefinitions, docPath string) {
	v := c.view
	defs := config.clone()
	defs.add(defs.plugins, v.keys(v.entryValue(body, nsPlugins))) // opened first
	c.defs = defs
	c.walkYAML(v.entryValue(body, nsVars))
	defs.add(defs.vars, v.keys(v.entryValue(body, nsVars)))
	c.walkYAML(v.entryValue(body, nsSecrets))
	defs.add(defs.secrets, v.keys(v.entryValue(body, nsSecrets)))

	for _, step := range v.sequence(v.entryValue(body, nsSteps)) {
		entries := v.entries(step)
		// The vars and secrets of a step are added for the step only and
		// removed after it: copying the definitions for each step made
		// scenarios of many steps quadratic.
		c.defs = defs
		c.walkYAML(v.entryValue(entries, "if"))
		c.walkYAML(v.entryValue(entries, nsVars))
		stepVars := defs.addNew(defs.vars, v.keys(v.entryValue(entries, nsVars)))
		c.walkYAML(v.entryValue(entries, nsSecrets))
		stepSecrets := defs.addNew(defs.secrets, v.keys(v.entryValue(entries, nsSecrets)))
		for _, n := range v.entryValues(entries, stepEvaluatedFields) {
			c.walkYAML(n)
		}

		// The bind of an include step reads the final context of the
		// included scenario: its steps instead of these ones, and its vars
		// on top of these.
		bind := v.entryValue(entries, "bind")
		if include := v.stringValue(v.entryValue(entries, "include")); include != "" {
			included, ok := templateDefinitions{}, false
			if docPath != "" {
				included, ok = s.includedNames(c, includedPath(docPath, include))
			}
			c.defs.steps = included.steps // c.defs is a copy of defs, sharing its sets
			c.bound = included
			c.unresolved = !ok
			c.walkYAML(bind)
			c.unresolved = false
			c.defs, c.bound = defs, templateDefinitions{}
		} else {
			c.walkYAML(bind)
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
	defs, ok := s.scenarioNames(path)
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
	defs.add(defs.vars, extractBlockKeys(configText, nsVars))
	defs.add(defs.secrets, extractBlockKeys(configText, nsSecrets))
	defs.add(defs.plugins, extractBlockKeys(configText, nsPlugins))
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
	includers func() templateDefinitions
	// unresolved is set where the names come from a file that cannot be
	// read, so that undefined names are not reported.
	unresolved bool
	// bound holds, in the bind of an include step, the names of the
	// included scenario: its vars and secrets are seen on top of defs,
	// without adding them to defs for each include step.
	bound templateDefinitions
	// included holds the names of the scenarios included so far, by path.
	included map[string]includedScenario
	// onPath, when set, is called with every name path found, such as
	// [vars user] for vars.user.name.
	onPath func(path []*tmplast.Ident, m valueMap)
}

// walkYAML checks the strings of a YAML subtree. Aliases are not followed:
// the anchored strings are checked where the anchor is, if at all, so that
// an anchor used twice is not reported twice.
func (c *templateChecker) walkYAML(node ast.Node) {
	switch n := node.(type) {
	case *ast.AnchorNode:
		c.walkYAML(n.Value)
	case *ast.TagNode:
		c.walkYAML(n.Value)
	case *ast.MappingNode:
		for _, mv := range n.Values {
			c.walkYAML(mv)
		}
	case *ast.MappingValueNode:
		c.walkYAML(n.Key) // left arrow functions and templated keys
		c.walkYAML(n.Value)
	case *ast.SequenceNode:
		for _, v := range n.Values {
			c.walkYAML(v)
		}
	case *ast.StringNode:
		// The offset of a string costs its column: computed for templates
		// only, a long line of plain values stays linear.
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

// checkString parses one YAML string value that starts at the byte offset
// start of the text.
func (c *templateChecker) checkString(value string, start int) {
	if start < 0 || !strings.Contains(value, "{{") {
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
		c.checkPath(expr, m, optional)
	case *tmplast.CallExpr:
		c.walkExpr(e.Fun, m, false)
		for _, arg := range e.Args {
			c.requireOperand(arg, e.Lparen, m)
		}
	case *tmplast.LeftArrowExpr:
		c.walkExpr(e.Fun, m, false)
		c.walkExpr(e.Arg, m, false) // no argument in a map key
	case *tmplast.ParameterExpr:
		c.walkExpr(e.X, m, false) // {{}} is an empty string
	case *tmplast.ParenExpr:
		c.requireOperand(e.X, e.Lparen, m)
	case *tmplast.UnaryExpr:
		c.requireOperandAt(e.X, e.OpPos, len(e.Op.String()), m)
	case *tmplast.BinaryExpr:
		if e.Op == tmpltoken.COALESCING && e.X != nil {
			c.walkExpr(e.X, m, isPathExpr(e.X))
		} else {
			c.requireOperandAt(e.X, e.OpPos, len(e.Op.String()), m)
		}
		c.requireOperandAt(e.Y, e.OpPos, len(e.Op.String()), m)
	case *tmplast.ConditionalExpr:
		c.requireOperand(e.Condition, e.Question, m)
		c.requireOperand(e.X, e.Question, m)
		c.requireOperand(e.Y, e.Colon, m)
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

func isPathExpr(expr tmplast.Expr) bool {
	switch expr.(type) {
	case *tmplast.Ident, *tmplast.SelectorExpr, *tmplast.IndexExpr:
		return true
	}
	return false
}

// templatePath is a path expression such as vars.user.name split into its
// identifiers from the root. An index ends the identifiers: vars[0].x yields
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
		p.closed = true
		p.indexes = append(p.indexes, e)
		return p
	}
	return templatePath{closed: true, base: expr}
}

func (c *templateChecker) checkPath(expr tmplast.Expr, m valueMap, optional bool) {
	p := pathOf(expr)
	for _, index := range p.indexes {
		c.requireOperand(index.Index, index.Lbrack, m)
	}
	if p.base != nil {
		c.walkExpr(p.base, m, false)
	}
	path := p.idents
	if len(path) == 0 {
		return
	}
	if c.onPath != nil {
		c.onPath(path, m)
	}
	root := path[0]
	if root.Name != templateActualValue && !slices.Contains(templateContextRoots, root.Name) && !slices.Contains(templateBuiltins, root.Name) {
		if !optional {
			c.report(m, root, DiagnosticSeverityWarning, fmt.Sprintf("unknown template name %q", root.Name))
		}
		return
	}
	if len(path) >= 2 {
		c.checkName(root.Name, path[1], m, optional)
	}
}

// checkName checks the name that follows a root, such as user in vars.user.
func (c *templateChecker) checkName(root string, name *tmplast.Ident, m valueMap, optional bool) {
	switch root {
	case "assert":
		if !slices.ContainsFunc(assertCandidates(), func(a templateCandidate) bool { return a.name == name.Name }) {
			c.report(m, name, DiagnosticSeverityWarning, fmt.Sprintf("unknown assertion %q", "assert."+name.Name))
		}
	case nsSteps:
		if !optional && !c.unresolved && !c.defs.steps[name.Name] {
			c.report(m, name, DiagnosticSeverityWarning, fmt.Sprintf("no step with the id %q has run before this point of the scenario", name.Name))
		}
	case nsPlugins:
		if !optional && !c.unresolved && !c.defs.plugins[name.Name] && !c.includers().plugins[name.Name] {
			c.report(m, name, DiagnosticSeverityWarning, fmt.Sprintf("plugin %q is not declared in this file, scenarigo.yaml or a scenario that includes this file", name.Name))
		}
	case nsVars, nsSecrets:
		if !optional && !c.unresolved && !c.defined(root, name.Name) {
			c.report(m, name, DiagnosticSeverityInformation, fmt.Sprintf("%s.%s is not defined before this point (a plugin setup may define it)", root, name.Name))
		}
	}
}

// defined reports whether a name of vars or secrets is defined: in scope,
// by the included scenario in the bind of an include step, or by a scenario
// that includes this file.
func (c *templateChecker) defined(root, name string) bool {
	return c.defs.set(root)[name] || c.bound.set(root)[name] || c.includers().set(root)[name]
}

func (c *templateChecker) report(m valueMap, ident *tmplast.Ident, severity int, msg string) {
	c.reportAt(m, ident.NamePos, utf8.RuneCountInString(ident.Name), severity, msg)
}

// reportAt reports a diagnostic on the runes [pos, pos+length) of the value,
// where pos is a 1-based parser position.
func (c *templateChecker) reportAt(m valueMap, pos, length, severity int, msg string) {
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
