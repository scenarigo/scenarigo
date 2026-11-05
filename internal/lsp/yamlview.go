package lsp

import (
	"iter"
	"slices"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"

	"github.com/scenarigo/scenarigo/internal/yamlschema"
)

// yamlView reads the mappings of a YAML file the way the loader sees them:
// aliases resolve to the anchors of their document and those before it, and
// merge keys (<<: *base) add the entries of the merged mappings. An alias to an
// anchor written later in its document resolves too, which the loader rejects.
type yamlView struct {
	anchors map[string]ast.Node
}

// maxAliasDepth bounds the aliases behind one value, so that an anchor that
// aliases itself cannot loop.
const maxAliasDepth = 16

// yamlDocuments returns the documents of a file, each with a view that reads
// it. The decoder of the loader keeps anchors from one document to the next,
// and an anchor of the same name replaces them, so a document sees those of
// the documents before it and its own. Read each document before going on.
func yamlDocuments(file *ast.File) iter.Seq2[*yamlView, *ast.DocumentNode] {
	return func(yield func(*yamlView, *ast.DocumentNode) bool) {
		v := &yamlView{anchors: map[string]ast.Node{}}
		for _, d := range file.Docs {
			ast.Walk(anchorCollector(v.anchors), d)
			if !yield(v, d) {
				return
			}
		}
	}
}

type anchorCollector map[string]ast.Node

func (a anchorCollector) Visit(node ast.Node) ast.Visitor {
	if n, ok := node.(*ast.AnchorNode); ok && n.Name != nil && n.Value != nil {
		a[n.Name.GetToken().Value] = n.Value
	}
	return a
}

// unwrap returns the node behind anchors, tags and aliases.
func (v *yamlView) unwrap(node ast.Node) ast.Node {
	for range maxAliasDepth {
		switch n := node.(type) {
		case *ast.AnchorNode:
			node = n.Value
		case *ast.TagNode:
			node = n.Value
		case *ast.AliasNode:
			if n.Value == nil || n.Value.GetToken() == nil {
				return nil
			}
			node = v.anchors[n.Value.GetToken().Value]
		default:
			return node
		}
	}
	return nil
}

// nameColumn returns the rune column of the text of tok, after its quote.
func nameColumn(tok *token.Token) int {
	if tok.Type == token.DoubleQuoteType || tok.Type == token.SingleQuoteType {
		return tok.Position.Column + 1
	}
	return tok.Position.Column
}

// entries returns the entries of a mapping node with what its merge keys
// bring, last written first, or nil when node is not a mapping. entryValue
// returns the first match, which is the value the loader gives the top-level
// map of vars or secrets: of a key and a merge key that brings it, the one
// written last wins. A nested mapping keeps both, and a step is rejected.
func (v *yamlView) entries(node ast.Node) []*ast.MappingValueNode {
	return v.expand(node, map[ast.Node]bool{})
}

// expand expands each mapping once, however many merge keys refer to it:
// that stops a cycle, and keeps <<: [*a, *a, *a] from growing exponentially
// with the depth of the merges.
func (v *yamlView) expand(node ast.Node, seen map[ast.Node]bool) []*ast.MappingValueNode {
	target := v.unwrap(node)
	if target == nil || seen[target] {
		return nil
	}
	seen[target] = true
	var values []*ast.MappingValueNode
	switch n := target.(type) {
	case *ast.MappingNode:
		values = n.Values
	case *ast.MappingValueNode:
		values = []*ast.MappingValueNode{n}
	default:
		return nil
	}
	var out []*ast.MappingValueNode
	for _, mv := range slices.Backward(values) {
		if _, ok := mv.Key.(*ast.MergeKeyNode); !ok {
			out = append(out, mv)
			continue
		}
		switch m := v.unwrap(mv.Value).(type) {
		case *ast.SequenceNode:
			for _, item := range slices.Backward(m.Values) {
				out = append(out, v.expand(item, seen)...)
			}
		default:
			out = append(out, v.expand(m, seen)...)
		}
	}
	return out
}

// keyName returns the name of a mapping key. Key.String() keeps the quotes
// of the key and the comment after it.
func keyName(key ast.MapKeyNode) string {
	if tok := key.GetToken(); tok != nil {
		return tok.Value
	}
	return key.String()
}

func (v *yamlView) entryValue(entries []*ast.MappingValueNode, key string) ast.Node {
	for _, mv := range entries {
		if mv.Key != nil && keyName(mv.Key) == key {
			return mv.Value
		}
	}
	return nil
}

func (v *yamlView) stringValue(node ast.Node) string {
	switch s := v.unwrap(node).(type) {
	case *ast.StringNode:
		return s.Value
	case *ast.IntegerNode, *ast.FloatNode, *ast.BoolNode: // id: 123 is the string "123"
		return s.GetToken().Value
	case *ast.LiteralNode: // | and > scalars
		if s.Value != nil {
			return s.Value.Value
		}
	}
	return ""
}

// isScenarigoDocument reports whether a file holds scenarigo documents. The
// lines answer for YAML that does not parse while it is being typed; the AST
// finds a schemaVersion they do not show, in a flow mapping or as a quoted key.
func isScenarigoDocument(doc *document) bool {
	if yamlschema.DetectSchemaType(doc.Text) != nil {
		return true
	}
	if doc.Parsed == nil || doc.Parsed.File == nil {
		return false
	}
	for v, d := range yamlDocuments(doc.Parsed.File) {
		if v.entryValue(v.entries(d.Body), "schemaVersion") != nil {
			return true
		}
	}
	return false
}

// documentFields returns the fields of one YAML document from its own
// schemaVersion, as the loader reads it (schema/load.go). A document without
// one is a scenario, also next to documents of another version, and needs no
// schemaVersion. An unknown version gets the scenario fields, as in
// yamlschema.DetectSchemaType.
func documentFields(v *yamlView, node ast.Node) []*yamlschema.FieldInfo {
	version := v.stringValue(v.entryValue(v.entries(node), "schemaVersion"))
	if sch := yamlschema.SchemaForVersion(version); sch != nil {
		return sch.Fields
	}
	fields := yamlschema.ScenarioSchema().Fields
	if version != "" {
		return fields
	}
	fields = slices.Clone(fields)
	for i, f := range fields {
		if f.Name == "schemaVersion" {
			optional := *f
			optional.Required = false
			fields[i] = &optional
		}
	}
	return fields
}
