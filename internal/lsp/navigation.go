package lsp

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/goccy/go-yaml/ast"

	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
)

// Helpers shared by definition, references and rename.

// nameChain is a dotted name chain found in a template expression, with its
// byte offsets in the expression.
type nameChain struct {
	path       []string
	start, end int
}

// isNameStart and isNamePart follow the identifiers of the template scanner
// (template/parser/scanner.go); $ is the actual value inside expect.
func isNameStart(r rune) bool {
	return r == '$' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || (r >= utf8.RuneSelf && unicode.IsLetter(r))
}

func isNamePart(r rune) bool {
	return isNameStart(r) || r == '-' || r == '_' || ('0' <= r && r <= '9') || (r >= utf8.RuneSelf && unicode.IsDigit(r))
}

// nameChains returns the dotted name chains of a template expression, such
// as vars.user in size(vars.user) ?? 0. Characters inside double-quoted
// strings are not names.
func nameChains(expr string) []nameChain {
	var chains []nameChain
	for i := 0; i < len(expr); {
		r, size := utf8.DecodeRuneInString(expr[i:])
		switch {
		case r == '"':
			end := strings.IndexByte(expr[i+1:], '"')
			if end < 0 {
				return chains
			}
			i += end + 2
		case isNameStart(r):
			start := i
			var path []string
			for {
				j := i
				for j < len(expr) {
					r, size := utf8.DecodeRuneInString(expr[j:])
					if !isNamePart(r) {
						break
					}
					j += size
				}
				path = append(path, expr[i:j])
				i = j
				if i+1 < len(expr) && expr[i] == '.' {
					if next, _ := utf8.DecodeRuneInString(expr[i+1:]); isNamePart(next) {
						i++
						continue
					}
				}
				break
			}
			if len(path) > 1 {
				chains = append(chains, nameChain{path: path, start: start, end: i})
			}
		case isNamePart(r):
			// The rest of a number or of a name the chain did not start with.
			for i < len(expr) {
				r, size := utf8.DecodeRuneInString(expr[i:])
				if !isNamePart(r) {
					break
				}
				i += size
			}
		default:
			i += size
		}
	}
	return chains
}

// chainAt returns the name chain of a template expression under the cursor.
func chainAt(text string, pos Position) (templateRef, bool) {
	for _, ref := range scanTemplateRefs(lineAt(text, pos.Line)) {
		if ref.startCol <= pos.Character && pos.Character <= ref.endCol {
			ref.line = pos.Line
			return ref, true
		}
	}
	return templateRef{}, false
}

// templateRefRanges returns the ranges of root.name in the template
// expressions of the text, as textDocument/references reports them.
func templateRefRanges(text, root, name string) []Range {
	var out []Range
	for _, ref := range scanTemplateRefs(text) {
		if len(ref.path) < 2 || ref.path[0] != root || ref.path[1] != name {
			continue
		}
		// Highlight just the matched prefix (e.g., "vars.myVar" portion).
		end := min(ref.startCol+len(root)+1+len(name), ref.endCol)
		out = append(out, Range{
			Start: Position{Line: ref.line, Character: ref.startCol},
			End:   Position{Line: ref.line, Character: end},
		})
	}
	return out
}

// plainScalar returns a scalar written after a colon without its quotes and
// trailing comment.
func plainScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		if end := strings.IndexByte(s[1:], s[0]); end >= 0 {
			return s[1 : end+1]
		}
	}
	s, _, _ = strings.Cut(s, " #")
	return strings.TrimSpace(s)
}

// docStartLines returns the 0-based line on which each YAML document of a
// file starts.
func docStartLines(file *ast.File) []int {
	starts := make([]int, len(file.Docs))
	for i, d := range file.Docs {
		first := -1
		if d.Body != nil {
			ast.Walk(visitFunc(func(n ast.Node) {
				if tok := n.GetToken(); tok != nil && tok.Position != nil && (first < 0 || tok.Position.Line-1 < first) {
					first = tok.Position.Line - 1
				}
			}), d.Body)
		}
		if first < 0 && d.Start != nil && d.Start.Position != nil {
			first = d.Start.Position.Line - 1 // an empty document starts at its ---
		}
		if first < 0 && i > 0 {
			first = starts[i-1]
		}
		starts[i] = max(first, 0)
	}
	return starts
}

// docIndexAt returns the index of the YAML document that contains a line.
func docIndexAt(starts []int, line int) int {
	idx := 0
	for i, start := range starts {
		if i > 0 && line >= start {
			idx = i
		}
	}
	return idx
}

type visitFunc func(ast.Node)

func (f visitFunc) Visit(n ast.Node) ast.Visitor {
	if n != nil {
		f(n)
	}
	return f
}

// nodeLines returns the first and last 0-based lines of the tokens of a
// node.
func nodeLines(node ast.Node) (int, int) {
	first, last := -1, -1
	ast.Walk(visitFunc(func(n ast.Node) {
		if tok := n.GetToken(); tok != nil && tok.Position != nil {
			line := tok.Position.Line - 1
			if first < 0 || line < first {
				first = line
			}
			last = max(last, line)
		}
	}), node)
	return first, last
}

// stepDefinition returns the location of the step id that steps.name at pos
// refers to: in the bind of an include step, a step of the included
// scenario; elsewhere, a step of the same YAML document.
func (s *Server) stepDefinition(doc *document, pos Position, name string) *Location {
	if doc.Parsed == nil || doc.Parsed.File == nil || doc.Parsed.Text != doc.Text {
		return nil
	}
	v := newYAMLView(doc.Parsed.File)
	starts := docStartLines(doc.Parsed.File)
	body := v.entries(doc.Parsed.File.Docs[docIndexAt(starts, pos.Line)].Body)
	for _, step := range v.sequence(v.entryValue(body, nsSteps)) {
		entries := v.entries(step)
		include := v.stringValue(v.entryValue(entries, "include"))
		bind := v.entryValue(entries, "bind")
		if include == "" || bind == nil {
			continue
		}
		if first, last := nodeLines(bind); first <= pos.Line && pos.Line <= last {
			return s.stepIDLocation(includedPath(uriToPath(doc.URI), include), name)
		}
	}
	lines := newLineIndex(doc.Text)
	for _, step := range v.sequence(v.entryValue(body, nsSteps)) {
		if id, ok := v.unwrap(v.entryValue(v.entries(step), "id")).(*ast.StringNode); ok && id.Value == name {
			if r, ok := scalarRange(doc.Text, lines, id); ok {
				return &Location{URI: doc.URI, Range: r}
			}
		}
	}
	return nil
}

// stepIDLocation returns the location of a step id in another file.
func (s *Server) stepIDLocation(path, name string) *Location {
	text, ok := s.readText(path)
	if !ok {
		return nil
	}
	parsed := yamlutil.Parse(text)
	if parsed == nil || parsed.File == nil {
		return nil
	}
	v := newYAMLView(parsed.File)
	lines := newLineIndex(text)
	for _, d := range parsed.File.Docs {
		for _, step := range v.sequence(v.entryValue(v.entries(d.Body), nsSteps)) {
			if id, ok := v.unwrap(v.entryValue(v.entries(step), "id")).(*ast.StringNode); ok && id.Value == name {
				if r, ok := scalarRange(text, lines, id); ok {
					return &Location{URI: pathToURI(path), Range: r}
				}
			}
		}
	}
	return nil
}
