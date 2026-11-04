package lsp

import (
	"slices"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"

	"github.com/scenarigo/scenarigo/internal/yamlschema"
)

// yamlView reads the mappings of a parsed YAML file the way the loader sees
// them: an alias resolves to the last anchor of its name before it (an
// anchor name can be defined again, also in another document), and merge
// keys (<<: *base) add the entries of the merged mappings.
type yamlView struct {
	anchors map[string][]anchorDef
}

type anchorDef struct {
	pos  *token.Position
	node ast.Node
}

// maxAliasDepth bounds alias and merge resolution, so that a cycle in a
// broken file cannot loop.
const maxAliasDepth = 16

// maxMergedEntries bounds the entries that the merge keys of a mapping
// bring in, which nested merges can multiply.
const maxMergedEntries = 10_000

func newYAMLView(file *ast.File) *yamlView {
	v := &yamlView{anchors: map[string][]anchorDef{}}
	if file == nil {
		return v
	}
	for _, d := range file.Docs {
		ast.Walk(anchorCollector(v.anchors), d)
	}
	return v
}

type anchorCollector map[string][]anchorDef

func (a anchorCollector) Visit(node ast.Node) ast.Visitor {
	if n, ok := node.(*ast.AnchorNode); ok && n.Name != nil && n.Value != nil {
		name := n.Name.GetToken().Value
		a[name] = append(a[name], anchorDef{pos: n.GetToken().Position, node: n.Value})
	}
	return a
}

// anchor returns the node that an alias names: the last anchor of its name
// before it. An alias without a position takes the last one of the file.
func (v *yamlView) anchor(alias *ast.AliasNode) ast.Node {
	if alias.Value == nil || alias.Value.GetToken() == nil {
		return nil
	}
	var pos *token.Position
	if tk := alias.GetToken(); tk != nil {
		pos = tk.Position
	}
	var found ast.Node
	for _, d := range v.anchors[alias.Value.GetToken().Value] {
		if pos == nil || d.pos == nil || d.pos.Line < pos.Line || d.pos.Line == pos.Line && d.pos.Column < pos.Column {
			found = d.node
		}
	}
	return found
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
			node = v.anchor(n)
		default:
			return node
		}
	}
	return nil
}

// entries returns the entries of a mapping node, followed by the entries
// its merge keys bring in, or nil when node is not a mapping. entryValue
// returns the first match, so explicit keys win over merged ones as in the
// loader.
func (v *yamlView) entries(node ast.Node) []*ast.MappingValueNode {
	return v.entriesDepth(node, 0)
}

func (v *yamlView) entriesDepth(node ast.Node, depth int) []*ast.MappingValueNode {
	if depth > maxAliasDepth {
		return nil
	}
	var values []*ast.MappingValueNode
	switch n := v.unwrap(node).(type) {
	case *ast.MappingNode:
		values = n.Values
	case *ast.MappingValueNode:
		values = []*ast.MappingValueNode{n}
	default:
		return nil
	}
	var own, merged []*ast.MappingValueNode
	for _, mv := range values {
		if _, ok := mv.Key.(*ast.MergeKeyNode); !ok {
			own = append(own, mv)
			continue
		}
		switch m := v.unwrap(mv.Value).(type) {
		case *ast.SequenceNode:
			for _, item := range m.Values {
				merged = append(merged, v.entriesDepth(item, depth+1)...)
			}
		default:
			merged = append(merged, v.entriesDepth(m, depth+1)...)
		}
		if len(merged) > maxMergedEntries {
			merged = merged[:maxMergedEntries] // merges of merges of the same mappings
			break
		}
	}
	return append(own, merged...)
}

// sequence returns the items of a sequence node, or nil.
func (v *yamlView) sequence(node ast.Node) []ast.Node {
	if s, ok := v.unwrap(node).(*ast.SequenceNode); ok {
		return s.Values
	}
	return nil
}

func (v *yamlView) entryValue(entries []*ast.MappingValueNode, key string) ast.Node {
	for _, mv := range entries {
		if mv.Key != nil && mv.Key.String() == key {
			return mv.Value
		}
	}
	return nil
}

// entryValues returns the values of the given keys, in document order.
func (v *yamlView) entryValues(entries []*ast.MappingValueNode, keys []string) []ast.Node {
	var out []ast.Node
	for _, mv := range entries {
		if mv.Key != nil && slices.Contains(keys, mv.Key.String()) && mv.Value != nil {
			out = append(out, mv.Value)
		}
	}
	return out
}

func (v *yamlView) stringValue(node ast.Node) string {
	if s, ok := v.unwrap(node).(*ast.StringNode); ok {
		return s.Value
	}
	return ""
}

// keys returns the keys of a mapping node.
func (v *yamlView) keys(node ast.Node) []string {
	var out []string
	for _, mv := range v.entries(node) {
		if mv.Key != nil {
			out = append(out, mv.Key.String())
		}
	}
	return out
}

// documentSchema returns the schema of one YAML document from its own
// schemaVersion, or nil when it has none the server knows.
func documentSchema(v *yamlView, body []*ast.MappingValueNode) *yamlschema.Schema {
	switch version := v.stringValue(v.entryValue(body, "schemaVersion")); version {
	case "":
		return nil
	default:
		return yamlschema.SchemaForVersion(version)
	}
}
