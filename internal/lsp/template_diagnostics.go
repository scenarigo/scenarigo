package lsp

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
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
// fail. Names are then checked against what the file and scenarigo.yaml
// define. Values that only exist at run time get a lower severity: a plugin's
// setup function can add variables with ctx.WithVars, so an undefined
// vars.x or secrets.x is only information.

// templateContextRoots are the names the template context resolves
// (context/extract.go).
var templateContextRoots = []string{"ctx", nsPlugins, nsVars, nsSecrets, nsSteps, "request", "response", "env", "assert"}

// templateBuiltins are the functions the template package resolves before
// the context: size, type, and the type conversions of template/val.
var templateBuiltins = []string{"size", "type", "any", "int", "uint", "float", "bool", "string", "bytes", "time", "duration"}

// templateActualValue is the value under test inside expect ({{$ != ""}}).
const templateActualValue = "$"

// Fields whose string values scenarigo evaluates as templates.
var (
	scenarioTemplateFields = []string{nsVars, nsSecrets, "anchors"}
	stepTemplateFields     = []string{nsVars, nsSecrets, "if", "request", "expect", "bind", "ref"}
	configTemplateFields   = []string{nsVars, nsSecrets}
)

// templateDefinitions holds the names a YAML document can refer to.
type templateDefinitions struct {
	vars, secrets, steps, plugins map[string]bool
}

func (s *Server) templateDiagnostics(doc *document) []Diagnostic {
	if doc.Parsed == nil || doc.Parsed.File == nil || doc.Parsed.Text != doc.Text {
		// The positions of a cached AST do not match the edited text.
		return nil
	}
	lines := newLineIndex(doc.Text)
	config := s.configDefinitions()
	var diags []Diagnostic
	for _, d := range doc.Parsed.File.Docs {
		body := mappingEntries(d.Body)
		if body == nil {
			continue
		}
		var roots []ast.Node
		defs := config.clone()
		// ytt/v1 documents hold file names only and fall through to the
		// scenario fields, none of which they have.
		switch stringValue(entryValue(body, "schemaVersion")) {
		case "config/v1":
			defs.addKeys(defs.vars, entryValue(body, nsVars))
			defs.addKeys(defs.secrets, entryValue(body, nsSecrets))
			roots = entryValues(body, configTemplateFields)
		default:
			defs.addScenario(body)
			roots = entryValues(body, scenarioTemplateFields)
			if steps, ok := unwrap(entryValue(body, nsSteps)).(*ast.SequenceNode); ok {
				for _, step := range steps.Values {
					roots = append(roots, entryValues(mappingEntries(step), stepTemplateFields)...)
				}
			}
		}
		c := &templateChecker{text: doc.Text, lines: lines, defs: defs, diags: &diags}
		for _, root := range roots {
			c.walkYAML(root)
		}
	}
	return diags
}

// configDefinitions returns the names scenarigo.yaml defines.
func (s *Server) configDefinitions() templateDefinitions {
	defs := templateDefinitions{vars: map[string]bool{}, secrets: map[string]bool{}, steps: map[string]bool{}, plugins: map[string]bool{}}
	configText, _, ok := s.readConfigText()
	if !ok {
		return defs
	}
	for _, block := range []struct {
		name string
		set  map[string]bool
	}{{nsVars, defs.vars}, {nsSecrets, defs.secrets}, {nsPlugins, defs.plugins}} {
		for _, key := range extractBlockKeys(configText, block.name) {
			block.set[key] = true
		}
	}
	return defs
}

func (d templateDefinitions) clone() templateDefinitions {
	return templateDefinitions{vars: cloneSet(d.vars), secrets: cloneSet(d.secrets), steps: cloneSet(d.steps), plugins: cloneSet(d.plugins)}
}

func cloneSet(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	maps.Copy(out, m)
	return out
}

// addScenario adds the names a scenario document defines: its vars, secrets
// and plugins, and the ids, vars, secrets and bound values of its steps.
func (d templateDefinitions) addScenario(body []*ast.MappingValueNode) {
	d.addKeys(d.vars, entryValue(body, nsVars))
	d.addKeys(d.secrets, entryValue(body, nsSecrets))
	d.addKeys(d.plugins, entryValue(body, nsPlugins))
	steps, ok := unwrap(entryValue(body, nsSteps)).(*ast.SequenceNode)
	if !ok {
		return
	}
	for _, item := range steps.Values {
		step := mappingEntries(item)
		if id := stringValue(entryValue(step, "id")); id != "" {
			d.steps[id] = true
		}
		d.addKeys(d.vars, entryValue(step, nsVars))
		d.addKeys(d.secrets, entryValue(step, nsSecrets))
		bind := mappingEntries(entryValue(step, "bind"))
		d.addKeys(d.vars, entryValue(bind, nsVars))
		d.addKeys(d.secrets, entryValue(bind, nsSecrets))
	}
}

func (d templateDefinitions) addKeys(set map[string]bool, node ast.Node) {
	for _, mv := range mappingEntries(node) {
		if mv.Key != nil {
			set[mv.Key.String()] = true
		}
	}
}

// unwrap returns the node behind anchors and tags.
func unwrap(node ast.Node) ast.Node {
	for {
		switch n := node.(type) {
		case *ast.AnchorNode:
			node = n.Value
		case *ast.TagNode:
			node = n.Value
		default:
			return node
		}
	}
}

// mappingEntries returns the entries of a mapping node, or nil.
func mappingEntries(node ast.Node) []*ast.MappingValueNode {
	switch n := unwrap(node).(type) {
	case *ast.MappingNode:
		return n.Values
	case *ast.MappingValueNode:
		return []*ast.MappingValueNode{n}
	}
	return nil
}

func entryValue(entries []*ast.MappingValueNode, key string) ast.Node {
	for _, mv := range entries {
		if mv.Key != nil && mv.Key.String() == key {
			return mv.Value
		}
	}
	return nil
}

func entryValues(entries []*ast.MappingValueNode, keys []string) []ast.Node {
	var out []ast.Node
	for _, mv := range entries {
		if mv.Key != nil && slices.Contains(keys, mv.Key.String()) && mv.Value != nil {
			out = append(out, mv.Value)
		}
	}
	return out
}

func stringValue(node ast.Node) string {
	if s, ok := unwrap(node).(*ast.StringNode); ok {
		return s.Value
	}
	return ""
}

// templateChecker reports the diagnostics of the template strings in a YAML
// subtree.
type templateChecker struct {
	text  string
	lines lineIndex
	defs  templateDefinitions
	diags *[]Diagnostic
}

func (c *templateChecker) walkYAML(node ast.Node) {
	switch n := unwrap(node).(type) {
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
		if tok := n.GetToken(); tok != nil {
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

// walkExpr checks the names used in expr. optional is set inside defined()
// and on the left of ??, where a missing value is expected.
func (c *templateChecker) walkExpr(expr tmplast.Expr, m valueMap, optional bool) {
	switch e := expr.(type) {
	case *tmplast.Ident, *tmplast.SelectorExpr:
		c.checkPath(expr, m, optional)
	case *tmplast.IndexExpr:
		c.checkPath(e, m, optional)
	case *tmplast.CallExpr:
		c.walkExpr(e.Fun, m, optional)
		for _, arg := range e.Args {
			c.walkExpr(arg, m, false)
		}
	case *tmplast.LeftArrowExpr:
		c.walkExpr(e.Fun, m, optional)
		c.walkExpr(e.Arg, m, false)
	case *tmplast.ParameterExpr:
		c.walkExpr(e.X, m, optional)
	case *tmplast.ParenExpr:
		c.walkExpr(e.X, m, optional)
	case *tmplast.UnaryExpr:
		c.walkExpr(e.X, m, optional)
	case *tmplast.BinaryExpr:
		c.walkExpr(e.X, m, optional || e.Op == tmpltoken.COALESCING)
		c.walkExpr(e.Y, m, optional)
	case *tmplast.ConditionalExpr:
		c.walkExpr(e.Condition, m, optional)
		c.walkExpr(e.X, m, optional)
		c.walkExpr(e.Y, m, optional)
	case *tmplast.DefinedExpr:
		c.walkExpr(e.Arg, m, true)
	}
}

// templatePath is a path expression such as vars.user.name split into its
// identifiers from the root. An index ends the identifiers: vars[0].x yields
// [vars]. indexes are the index expressions along the path, and base is the
// expression the path starts from when it is not an identifier (f().x).
type templatePath struct {
	idents  []*tmplast.Ident
	closed  bool
	indexes []tmplast.Expr
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
		p.indexes = append(p.indexes, e.Index)
		return p
	}
	return templatePath{closed: true, base: expr}
}

func (c *templateChecker) checkPath(expr tmplast.Expr, m valueMap, optional bool) {
	p := pathOf(expr)
	for _, index := range p.indexes {
		c.walkExpr(index, m, false)
	}
	if p.base != nil {
		c.walkExpr(p.base, m, optional)
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
		if !slices.ContainsFunc(assertCandidates(), func(a templateCandidate) bool { return a.name == name.Name }) {
			c.report(m, name, DiagnosticSeverityWarning, fmt.Sprintf("unknown assertion %q", "assert."+name.Name))
		}
	case nsSteps:
		if !optional && !c.defs.steps[name.Name] {
			c.report(m, name, DiagnosticSeverityWarning, fmt.Sprintf("no step has the id %q in this scenario", name.Name))
		}
	case nsPlugins:
		if !optional && !c.defs.plugins[name.Name] {
			c.report(m, name, DiagnosticSeverityWarning, fmt.Sprintf("plugin %q is not declared in this file or scenarigo.yaml", name.Name))
		}
	case nsVars, nsSecrets:
		set := c.defs.vars
		if root.Name == nsSecrets {
			set = c.defs.secrets
		}
		if !optional && !set[name.Name] {
			c.report(m, name, DiagnosticSeverityInformation, fmt.Sprintf("%s.%s is not defined in this file or scenarigo.yaml (a plugin setup may define it)", root.Name, name.Name))
		}
	}
}

func (c *templateChecker) report(m valueMap, ident *tmplast.Ident, severity int, msg string) {
	start := ident.NamePos - 1
	*c.diags = append(*c.diags, Diagnostic{
		Range:    m.runeRange(start, start+utf8.RuneCountInString(ident.Name)),
		Severity: severity,
		Message:  msg,
	})
}

// lineIndex holds the byte offset at which each line of a text starts.
type lineIndex []int

func newLineIndex(text string) lineIndex {
	idx := lineIndex{0}
	for i := range len(text) {
		if text[i] == '\n' {
			idx = append(idx, i+1)
		}
	}
	return idx
}

// lineStart returns the byte offset of the start of a 0-based line, or -1.
func (idx lineIndex) lineStart(line int) int {
	if line < 0 || line >= len(idx) {
		return -1
	}
	return idx[line]
}

// offset converts a goccy/go-yaml position (1-based line, 1-based rune
// column) into a byte offset of text, or -1.
func (idx lineIndex) offset(text string, line, runeCol int) int {
	start := idx.lineStart(line - 1)
	if start < 0 {
		return -1
	}
	return start + byteColumnFromRune(lineAt(text, line-1), runeCol-1)
}

// position converts a byte offset into an internal position.
func (idx lineIndex) position(offset int) Position {
	line := sort.Search(len(idx), func(i int) bool { return idx[i] > offset }) - 1
	line = max(line, 0)
	return Position{Line: line, Character: offset - idx[line]}
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
		end.Character += len(lineAt(m.text, start.Line)) - start.Character
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
