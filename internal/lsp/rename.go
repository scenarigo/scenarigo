package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml/ast"

	tmplast "github.com/scenarigo/scenarigo/template/ast"
)

// Rename of vars, secrets and step ids.
//
// The edits replace the name in every declaration of the YAML document and
// in every range textDocument/references finds in it. A rename that could
// leave a reference behind or change another name is refused rather than
// applied:
//   - references are found by scanning the text, so they are checked against
//     the template parser, and a name used as an index (vars["user"]) is
//     refused;
//   - a name declared through an alias or a merge key is declared somewhere
//     the rename does not edit;
//   - names that other files can use are not renamed from one file: names of
//     a configuration file, names scenarigo.yaml also defines, the vars of a
//     scenario that includes another one, and the names of a scenario that
//     another one includes.

// codeRequestFailed is the LSP error code for a valid request that failed.
const codeRequestFailed = -32803

// renameKeywords are words the template scanner does not read as names.
var renameKeywords = []string{"nil", "defined"}

// yamlKeywords are words YAML reads as a bool or null in any of their
// spellings (True, NULL), so they cannot be a key or an id.
var yamlKeywords = []string{"true", "false", "null"}

// validName reports whether a template can refer to name after a dot and
// YAML reads it as a string.
func validName(name string) bool {
	if name == "" || slices.Contains(renameKeywords, name) ||
		slices.ContainsFunc(yamlKeywords, func(k string) bool { return strings.EqualFold(k, name) }) {
		return false
	}
	for i, r := range name {
		if r == '$' || (i == 0 && !isNameStart(r)) || !isNamePart(r) {
			return false
		}
	}
	return true
}

type renamePlan struct {
	root, name string
	target     Range   // the name under the cursor
	ranges     []Range // the names to replace
	declared   map[string]bool
	used       map[string]bool // names of the namespace the templates refer to
}

func (s *Server) handlePrepareRename(req *Request) {
	var params PrepareRenameParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendResponse(req.ID, nil, invalidParams("prepareRename", err))
		return
	}
	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}
	plan, err := s.planRename(doc, s.decodePosition(doc.Text, params.Position))
	switch {
	case err != nil:
		s.sendResponse(req.ID, nil, &ResponseError{Code: codeRequestFailed, Message: err.Error()})
	case plan == nil:
		s.sendResponse(req.ID, nil, nil)
	default:
		s.sendResponse(req.ID, PrepareRenameResult{Range: s.encodeRange(doc.Text, plan.target), Placeholder: plan.name}, nil)
	}
}

func (s *Server) handleRename(req *Request) {
	var params RenameParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendResponse(req.ID, nil, invalidParams("rename", err))
		return
	}
	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(req.ID, nil, &ResponseError{Code: codeRequestFailed, Message: "the document is not open"})
		return
	}
	plan, err := s.planRename(doc, s.decodePosition(doc.Text, params.Position))
	if err == nil && plan == nil {
		err = errors.New("there is no variable, secret or step id to rename here")
	}
	if err == nil {
		err = plan.check(params.NewName)
	}
	if err != nil {
		s.sendResponse(req.ID, nil, &ResponseError{Code: codeRequestFailed, Message: err.Error()})
		return
	}
	edits := make([]TextEdit, 0, len(plan.ranges))
	for _, r := range plan.ranges {
		edits = append(edits, TextEdit{Range: s.encodeRange(doc.Text, r), NewText: params.NewName})
	}
	s.sendResponse(req.ID, WorkspaceEdit{Changes: map[string][]TextEdit{doc.URI: edits}}, nil)
}

// check validates the new name.
func (p *renamePlan) check(newName string) error {
	if !validName(newName) {
		return fmt.Errorf("%q is not a valid name: use letters, digits, - and _, starting with a letter, and not true, false, nil, null or defined", newName)
	}
	if newName != p.name && p.declared[newName] {
		return fmt.Errorf("%s.%s is already defined in this scenario or scenarigo.yaml", p.root, newName)
	}
	if newName != p.name && p.used[newName] {
		// A template that tests it with ?? or defined() would change meaning.
		return fmt.Errorf("%s.%s is already used in a template of this scenario", p.root, newName)
	}
	return nil
}

// renameDoc is a YAML document of the file, with its kind and the lines it
// spans.
type renameDoc struct {
	body        []*ast.MappingValueNode
	version     string
	first, last int // 0-based lines; last is -1 for the last document
}

func (d renameDoc) contains(line int) bool {
	return line >= d.first && (d.last < 0 || line <= d.last)
}

// planRename finds the symbol at pos and every range to replace in the YAML
// document at pos. It returns nil without an error when there is nothing to
// rename at pos, and an error when a rename would be incomplete.
func (s *Server) planRename(doc *document, pos Position) (*renamePlan, error) {
	if doc.Parsed == nil || doc.Parsed.File == nil || doc.Parsed.Text != doc.Text {
		return nil, nil //nolint:nilnil // nothing to rename in text that does not parse
	}
	v := newYAMLView(doc.Parsed.File)
	lines := newLineIndex(doc.Text)
	d := renameDocAt(doc, v, pos.Line)
	if d.body == nil || d.version == "ytt/v1" {
		return nil, nil //nolint:nilnil // nothing to rename
	}
	sym, ok := renameTarget(doc, v, lines, d, pos)
	if !ok {
		return nil, nil //nolint:nilnil // nothing to rename
	}
	root, name := sym.root, sym.name
	if d.version == configSchemaVersion {
		return nil, fmt.Errorf("cannot rename %s.%s: the names of a configuration file can be used by every scenario", root, name)
	}
	if s.rootURI == "" {
		return nil, fmt.Errorf("cannot rename %s.%s: without a workspace root, the scenarios that include this one cannot be checked", root, name)
	}
	if err := s.checkRenameScope(doc, v, lines, d, root, name); err != nil {
		return nil, err
	}

	config := s.configDefinitions()
	plan := newRenamePlan(doc, v, d, sym, config)

	// Declarations, then the references the text scan finds.
	direct := false
	for _, decl := range declarations(doc.Text, v, lines, d, root) {
		if decl.name == name {
			plan.ranges = appendRange(plan.ranges, decl.rng)
			direct = true
		}
	}
	if (!direct && plan.declared[name] && !config.set(root)[name]) || slices.Contains(mergedNames(v, d, root), name) {
		return nil, fmt.Errorf("cannot rename %s.%s: it is declared through an alias or merge key", root, name)
	}
	for _, r := range templateRefRanges(doc.Text, root, name) {
		if d.contains(r.Start.Line) {
			r.Start.Character += len(root) + 1
			r.End.Character = r.Start.Character + len(name)
			plan.ranges = appendRange(plan.ranges, r)
		}
	}

	if err := s.checkParsedReferences(doc, d, plan); err != nil {
		return nil, err
	}
	if !slices.Contains(plan.ranges, plan.target) {
		return nil, fmt.Errorf("cannot rename %s.%s: the name under the cursor cannot be located for renaming", root, name)
	}
	slices.SortFunc(plan.ranges, compareRanges)
	return plan, nil
}

// newRenamePlan returns a plan for sym, with the names declared in the
// document and scenarigo.yaml and the names the templates refer to.
func newRenamePlan(doc *document, v *yamlView, d renameDoc, sym renameSymbol, config templateDefinitions) *renamePlan {
	plan := &renamePlan{root: sym.root, name: sym.name, target: sym.target, declared: map[string]bool{}, used: map[string]bool{}}
	for _, ref := range scanTemplateRefs(doc.Text) {
		if len(ref.path) >= 2 && ref.path[0] == sym.root && d.contains(ref.line) {
			plan.used[ref.path[1]] = true
		}
	}
	for n := range config.set(sym.root) {
		plan.declared[n] = true
	}
	for _, n := range resolvedNames(v, d, sym.root) {
		plan.declared[n] = true
	}
	return plan
}

// renameDocAt returns the YAML document of the file that contains a line.
func renameDocAt(doc *document, v *yamlView, line int) renameDoc {
	starts := docStartLines(doc.Parsed.File)
	i := docIndexAt(starts, line)
	d := renameDoc{body: v.entries(doc.Parsed.File.Docs[i].Body), last: -1}
	if i > 0 {
		d.first = starts[i]
	}
	if i+1 < len(starts) {
		d.last = starts[i+1] - 1
	}
	d.version = v.stringValue(v.entryValue(d.body, "schemaVersion"))
	return d
}

// checkParsedReferences checks that every reference the template parser
// sees in the document is among the ranges of the plan.
func (s *Server) checkParsedReferences(doc *document, d renameDoc, plan *renamePlan) error {
	root, name := plan.root, plan.name
	var parsed []Range
	indexed := false
	var discard []Diagnostic
	s.walkTemplates(doc, &discard, templateHooks{
		onPath: nameCollector(root, name, &parsed),
		onIndex: func(r, key string, m valueMap) {
			indexed = indexed || (r == root && key == name)
		},
	})
	if indexed {
		return fmt.Errorf("cannot rename %s.%s: it is also used as %s[%q], which is not renamed", root, name, root, name)
	}
	for _, r := range parsed {
		if d.contains(r.Start.Line) && !slices.Contains(plan.ranges, r) {
			return fmt.Errorf("cannot rename %s.%s: the reference on line %d cannot be located for renaming (a template that spans lines); edit it by hand", root, name, r.Start.Line+1)
		}
	}
	return nil
}

// nameCollector returns an onPath hook that collects the ranges of name in
// the root.name paths.
func nameCollector(root, name string, out *[]Range) func(path []*tmplast.Ident, m valueMap) {
	return func(path []*tmplast.Ident, m valueMap) {
		if len(path) >= 2 && path[0].Name == root && path[1].Name == name {
			start := path[1].NamePos - 1
			*out = append(*out, m.runeRange(start, start+utf8.RuneCountInString(name)))
		}
	}
}

// resolvedNames returns the names a scenario document declares in a
// namespace, aliases and merge keys included.
func resolvedNames(v *yamlView, d renameDoc, root string) []string {
	if root == nsSteps {
		var ids []string
		for _, step := range v.sequence(v.entryValue(d.body, nsSteps)) {
			if id := v.stringValue(v.entryValue(v.entries(step), "id")); id != "" {
				ids = append(ids, id)
			}
		}
		return ids
	}
	names := v.keys(v.entryValue(d.body, root))
	for _, step := range v.sequence(v.entryValue(d.body, nsSteps)) {
		entries := v.entries(step)
		names = append(names, v.keys(v.entryValue(entries, root))...)
		names = append(names, v.keys(v.entryValue(v.entries(v.entryValue(entries, "bind")), root))...)
	}
	return names
}

// mergedNames returns the names of vars or secrets mappings of a scenario
// document that come from an alias or a merge key rather than from the
// mapping itself.
func mergedNames(v *yamlView, d renameDoc, root string) []string {
	if root == nsSteps {
		return nil
	}
	nodes := []ast.Node{v.entryValue(d.body, root)}
	for _, step := range v.sequence(v.entryValue(d.body, nsSteps)) {
		entries := v.entries(step)
		nodes = append(nodes, v.entryValue(entries, root), v.entryValue(v.entries(v.entryValue(entries, "bind")), root))
	}
	var merged []string
	for _, node := range nodes {
		own := map[string]bool{}
		if m, ok := unwrapAnchor(node).(*ast.MappingNode); ok {
			for _, mv := range m.Values {
				if _, isMerge := mv.Key.(*ast.MergeKeyNode); !isMerge && mv.Key != nil {
					own[mv.Key.String()] = true
				}
			}
		} else if mv, ok := unwrapAnchor(node).(*ast.MappingValueNode); ok && mv.Key != nil {
			own[mv.Key.String()] = true
		}
		for _, k := range v.keys(node) {
			if !own[k] {
				merged = append(merged, k)
			}
		}
	}
	return merged
}

// unwrapAnchor returns the node behind anchors and tags, but not aliases.
func unwrapAnchor(node ast.Node) ast.Node {
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

// checkRenameScope refuses names that other files can refer to.
func (s *Server) checkRenameScope(doc *document, v *yamlView, lines lineIndex, d renameDoc, root, name string) error {
	if (root == nsVars || root == nsSecrets) && s.configDefinitions().set(root)[name] {
		return fmt.Errorf("cannot rename %s.%s: scenarigo.yaml also defines it, and other scenarios can use it", root, name)
	}
	for _, step := range v.sequence(v.entryValue(d.body, nsSteps)) {
		entries := v.entries(step)
		include := v.stringValue(v.entryValue(entries, "include"))
		if include == "" {
			continue
		}
		if root == nsVars || root == nsSecrets {
			return fmt.Errorf("cannot rename %s.%s: this scenario includes another scenario (%s), which can use its %s", root, name, include, root)
		}
		// The bind of an include step refers to the steps of the included scenario.
		var found []Range
		var discard []Diagnostic
		c := &templateChecker{
			text: doc.Text, lines: lines, view: v, diags: &discard,
			defs:      newTemplateDefinitions(),
			includers: newTemplateDefinitions,
			hooks:     templateHooks{onPath: nameCollector(root, name, &found)},
		}
		c.walkYAML(v.entryValue(entries, "bind"))
		if len(found) > 0 {
			return fmt.Errorf("cannot rename steps.%s: the bind of an include step refers to a step of the included scenario with the same id", name)
		}
	}
	if includers := s.findIncluders(uriToPath(doc.URI)); len(includers) > 0 {
		rel, err := filepath.Rel(uriToPath(s.rootURI), includers[0])
		if err != nil {
			rel = includers[0]
		}
		return fmt.Errorf("cannot rename %s.%s: %s includes this scenario and can refer to its %s", root, name, rel, root)
	}
	return nil
}

// renameSymbol is a vars, secrets or steps name and the range of the name.
type renameSymbol struct {
	root, name string
	target     Range
}

// renameTarget returns the symbol under the cursor.
func renameTarget(doc *document, v *yamlView, lines lineIndex, d renameDoc, pos Position) (renameSymbol, bool) {
	if _, inTemplate := getFullTemplateExpr(doc.Text, pos); inTemplate {
		ref, ok := chainAt(doc.Text, pos)
		if !ok || (ref.path[0] != nsVars && ref.path[0] != nsSecrets && ref.path[0] != nsSteps) {
			return renameSymbol{}, false
		}
		root := ref.path[0]
		start := Position{Line: pos.Line, Character: ref.startCol + len(root) + 1}
		end := Position{Line: pos.Line, Character: start.Character + len(ref.path[1])}
		return renameSymbol{root: root, name: ref.path[1], target: Range{Start: start, End: end}}, true
	}
	within := func(r Range) bool {
		return r.Start.Line == pos.Line && r.Start.Character <= pos.Character && pos.Character <= r.End.Character
	}
	for _, r := range []string{nsVars, nsSecrets, nsSteps} {
		for _, decl := range declarations(doc.Text, v, lines, d, r) {
			if within(decl.rng) || (decl.key != nil && within(*decl.key)) {
				return renameSymbol{root: r, name: decl.name, target: decl.rng}, true
			}
		}
	}
	return renameSymbol{}, false
}

type declaration struct {
	name string
	rng  Range
	key  *Range // the "id" key of a step id, which also selects it
}

// declarations returns the declarations of a namespace written in a
// scenario document: the keys of vars or secrets at the top, in steps and in
// bind, or the ids of the steps. Keys brought by aliases and merge keys are
// not among them.
func declarations(text string, v *yamlView, lines lineIndex, d renameDoc, root string) []declaration {
	var out []declaration
	addKeys := func(node ast.Node) {
		var values []*ast.MappingValueNode
		switch n := node.(type) {
		case *ast.MappingNode:
			values = n.Values
		case *ast.MappingValueNode:
			values = []*ast.MappingValueNode{n}
		}
		for _, mv := range values {
			if key, ok := mv.Key.(*ast.StringNode); ok {
				if r, ok := scalarRange(text, lines, key); ok {
					out = append(out, declaration{name: key.Value, rng: r})
				}
			}
		}
	}
	direct := func(entries []*ast.MappingValueNode, key string) *ast.MappingValueNode {
		for _, mv := range entries {
			if mv.Key != nil && mv.Key.String() == key {
				return mv
			}
		}
		return nil
	}
	value := func(mv *ast.MappingValueNode) ast.Node {
		if mv == nil {
			return nil
		}
		return mv.Value
	}
	steps := v.sequence(v.entryValue(d.body, nsSteps))
	if root == nsSteps {
		for _, step := range steps {
			mv := direct(v.entries(step), "id")
			if mv == nil {
				continue
			}
			id, ok := mv.Value.(*ast.StringNode)
			if !ok {
				continue
			}
			r, ok := scalarRange(text, lines, id)
			if !ok {
				continue
			}
			decl := declaration{name: id.Value, rng: r}
			if key, ok := mv.Key.(*ast.StringNode); ok {
				if kr, ok := scalarRange(text, lines, key); ok {
					decl.key = &kr
				}
			}
			out = append(out, decl)
		}
		return out
	}
	addKeys(value(direct(d.body, root)))
	for _, step := range steps {
		entries := v.entries(step)
		addKeys(value(direct(entries, root)))
		addKeys(value(direct(v.entries(value(direct(entries, "bind"))), root)))
	}
	return out
}

// scalarRange returns the range of the value of a single-line string node,
// without its quotes.
func scalarRange(text string, lines lineIndex, node *ast.StringNode) (Range, bool) {
	tok := node.GetToken()
	if tok == nil {
		return Range{}, false
	}
	m := newValueMap(text, lines, lines.offset(text, tok.Position.Line, tok.Position.Column), node.Value)
	if len(m.offsets) == 0 {
		return Range{}, false
	}
	r := m.runeRange(0, len(m.offsets)-1)
	return r, r.Start.Line == r.End.Line
}

func appendRange(ranges []Range, r Range) []Range {
	if slices.Contains(ranges, r) {
		return ranges
	}
	return append(ranges, r)
}

func compareRanges(a, b Range) int {
	if a.Start.Line != b.Start.Line {
		return a.Start.Line - b.Start.Line
	}
	return a.Start.Character - b.Start.Character
}
