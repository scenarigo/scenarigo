package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/goccy/go-yaml/ast"

	tmplast "github.com/scenarigo/scenarigo/template/ast"
)

// Rename of vars, secrets and step ids.
//
// The edits replace the name in every declaration of the file and in every
// range textDocument/references finds. A rename that could leave a reference
// behind is refused rather than applied: references are found by scanning
// the text, so they are checked against the template parser, and names that
// other files can use (scenarigo.yaml, included and including scenarios) are
// not renamed from one file.

// codeRequestFailed is the LSP error code for a valid request that failed.
const codeRequestFailed = -32803

// renameNamePattern matches the names a template can refer to with a dot.
var renameNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

type renamePlan struct {
	root, name string
	target     Range   // the name under the cursor
	ranges     []Range // the names to replace
	declared   map[string]bool
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
	if !renameNamePattern.MatchString(newName) {
		return fmt.Errorf("%q is not a valid name: use letters, digits, - and _, starting with a letter or _", newName)
	}
	if newName != p.name && p.declared[newName] {
		return fmt.Errorf("%s.%s is already declared in this file", p.root, newName)
	}
	return nil
}

// renameDoc is a YAML document of the file, with its kind.
type renameDoc struct {
	body     []*ast.MappingValueNode
	scenario bool
}

// planRename finds the symbol at pos and every range to replace. It returns
// nil without an error when there is nothing to rename at pos, and an error
// when a rename would be incomplete.
func (s *Server) planRename(doc *document, pos Position) (*renamePlan, error) {
	if doc.Parsed == nil || doc.Parsed.File == nil || doc.Parsed.Text != doc.Text {
		return nil, nil //nolint:nilnil // nothing to rename in text that does not parse
	}
	v := newYAMLView(doc.Parsed.File)
	lines := newLineIndex(doc.Text)
	var docs []renameDoc
	for _, d := range doc.Parsed.File.Docs {
		if body := v.entries(d.Body); body != nil {
			version := v.stringValue(v.entryValue(body, "schemaVersion"))
			if version == "ytt/v1" {
				continue
			}
			docs = append(docs, renameDoc{body: body, scenario: version != "config/v1"})
		}
	}
	sym, ok := renameTarget(doc, v, lines, docs, pos)
	if !ok {
		return nil, nil //nolint:nilnil // nothing to rename
	}
	root, name := sym.root, sym.name
	plan := &renamePlan{root: root, name: name, target: sym.target, declared: map[string]bool{}}

	if err := s.checkRenameScope(doc, v, lines, docs, root, name); err != nil {
		return nil, err
	}

	// Declarations, then the references the text scan finds.
	for _, d := range docs {
		for _, decl := range declarations(doc.Text, v, lines, d, root) {
			plan.declared[decl.name] = true
			if decl.name == name {
				plan.ranges = appendRange(plan.ranges, decl.rng)
			}
		}
	}
	locs := s.references(doc, ReferenceParams{
		TextDocument: TextDocumentIdentifier{URI: doc.URI},
		Position:     pos,
	})
	for _, loc := range locs {
		start := loc.Range.Start
		start.Character += len(root) + 1
		end := start
		end.Character += len(name)
		plan.ranges = appendRange(plan.ranges, Range{Start: start, End: end})
	}

	// Every reference the template parser sees must be among them.
	var parsed []Range
	var discard []Diagnostic
	s.walkTemplates(doc, &discard, nameCollector(root, name, &parsed))
	for _, r := range parsed {
		if !slices.Contains(plan.ranges, r) {
			return nil, fmt.Errorf("cannot rename %s.%s: the reference on line %d cannot be located for renaming (for example, a template that spans lines); edit it by hand", root, name, r.Start.Line+1)
		}
	}
	if !slices.Contains(plan.ranges, plan.target) {
		return nil, fmt.Errorf("cannot rename %s.%s: the name under the cursor cannot be located for renaming", root, name)
	}
	slices.SortFunc(plan.ranges, compareRanges)
	return plan, nil
}

// nameCollector returns an onPath callback that collects the ranges of name
// in the root.name paths.
func nameCollector(root, name string, out *[]Range) func(path []*tmplast.Ident, m valueMap) {
	return func(path []*tmplast.Ident, m valueMap) {
		if len(path) >= 2 && path[0].Name == root && path[1].Name == name {
			start := path[1].NamePos - 1
			*out = append(*out, m.runeRange(start, start+len([]rune(name))))
		}
	}
}

// checkRenameScope refuses names that other files can refer to.
func (s *Server) checkRenameScope(doc *document, v *yamlView, lines lineIndex, docs []renameDoc, root, name string) error {
	if root == nsVars || root == nsSecrets {
		if s.configDefinitions().set(root)[name] {
			return fmt.Errorf("cannot rename %s.%s: scenarigo.yaml also defines it, and other scenarios can use it", root, name)
		}
	}
	for _, d := range docs {
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
				onPath:    nameCollector(root, name, &found),
			}
			c.walkYAML(v.entryValue(entries, "bind"))
			if len(found) > 0 {
				return fmt.Errorf("cannot rename steps.%s: the bind of an include step refers to a step of the included scenario with the same id", name)
			}
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
func renameTarget(doc *document, v *yamlView, lines lineIndex, docs []renameDoc, pos Position) (renameSymbol, bool) {
	if _, inTemplate := getFullTemplateExpr(doc.Text, pos); inTemplate {
		for _, ref := range scanTemplateRefs(lineAt(doc.Text, pos.Line)) {
			if pos.Character < ref.startCol || pos.Character > ref.endCol {
				continue
			}
			root := ref.path[0]
			if root != nsVars && root != nsSecrets && root != nsSteps {
				return renameSymbol{}, false
			}
			start := Position{Line: pos.Line, Character: ref.startCol + len(root) + 1}
			end := Position{Line: pos.Line, Character: start.Character + len(ref.path[1])}
			return renameSymbol{root: root, name: ref.path[1], target: Range{Start: start, End: end}}, true
		}
		return renameSymbol{}, false
	}
	for _, d := range docs {
		for _, r := range []string{nsVars, nsSecrets, nsSteps} {
			for _, decl := range declarations(doc.Text, v, lines, d, r) {
				if decl.rng.Start.Line == pos.Line && (decl.onLine || (decl.rng.Start.Character <= pos.Character && pos.Character <= decl.rng.End.Character)) {
					return renameSymbol{root: r, name: decl.name, target: decl.rng}, true
				}
			}
		}
	}
	return renameSymbol{}, false
}

type declaration struct {
	name   string
	rng    Range
	onLine bool // the whole line selects it (the "id:" key of a step id)
}

// declarations returns the declarations of a namespace in a YAML document:
// the keys of vars or secrets at the top, in steps and in bind, or the ids of
// the steps. Keys brought by aliases and merge keys are declared elsewhere
// and are not included.
func declarations(text string, v *yamlView, lines lineIndex, d renameDoc, root string) []declaration {
	var out []declaration
	addKeys := func(node ast.Node) {
		m, ok := node.(*ast.MappingNode)
		if !ok {
			if mv, ok := node.(*ast.MappingValueNode); ok {
				m = &ast.MappingNode{Values: []*ast.MappingValueNode{mv}}
			} else {
				return
			}
		}
		for _, mv := range m.Values {
			if key, ok := mv.Key.(*ast.StringNode); ok {
				if r, ok := scalarRange(text, lines, key); ok {
					out = append(out, declaration{name: key.Value, rng: r})
				}
			}
		}
	}
	direct := func(entries []*ast.MappingValueNode, key string) ast.Node {
		for _, mv := range entries {
			if mv.Key != nil && mv.Key.String() == key {
				return mv.Value
			}
		}
		return nil
	}
	if !d.scenario {
		if root != nsSteps {
			addKeys(direct(d.body, root))
		}
		return out
	}
	steps := v.sequence(v.entryValue(d.body, nsSteps))
	if root == nsSteps {
		for _, step := range steps {
			if id, ok := direct(v.entries(step), "id").(*ast.StringNode); ok {
				if r, ok := scalarRange(text, lines, id); ok {
					out = append(out, declaration{name: id.Value, rng: r, onLine: true})
				}
			}
		}
		return out
	}
	addKeys(direct(d.body, root))
	for _, step := range steps {
		entries := v.entries(step)
		addKeys(direct(entries, root))
		addKeys(direct(v.entries(direct(entries, "bind")), root))
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
