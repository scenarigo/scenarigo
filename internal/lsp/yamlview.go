package lsp

import (
	"slices"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"

	"github.com/scenarigo/scenarigo/internal/yamlschema"
)

// yamlView reads the mappings of a parsed YAML file the way the loader sees
// them: an alias resolves to the last anchor of its name in its document, or
// else in the documents before it, and merge keys (<<: *base) add the
// entries of the merged mappings.
type yamlView struct {
	anchors   map[string][]anchorDef
	aliasDocs map[*ast.AliasNode]int
	merged    map[ast.Node][]*ast.MappingValueNode // entries, by mapping
}

type anchorDef struct {
	doc  int
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
	v := &yamlView{
		anchors:   map[string][]anchorDef{},
		aliasDocs: map[*ast.AliasNode]int{},
		merged:    map[ast.Node][]*ast.MappingValueNode{},
	}
	if file == nil {
		return v
	}
	for i, d := range file.Docs {
		ast.Walk(&anchorCollector{view: v, doc: i}, d)
	}
	return v
}

type anchorCollector struct {
	view *yamlView
	doc  int
}

func (a *anchorCollector) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.AnchorNode:
		if n.Name != nil && n.Value != nil {
			name := n.Name.GetToken().Value
			a.view.anchors[name] = append(a.view.anchors[name], anchorDef{doc: a.doc, pos: n.GetToken().Position, node: n.Value})
		}
	case *ast.AliasNode:
		a.view.aliasDocs[n] = a.doc
	}
	return a
}

// anchor returns the node that an alias names, as scenarigo's loader reads
// it: the last anchor of its name in the alias's document, even after the
// alias, or else the last one in the documents before it.
func (v *yamlView) anchor(alias *ast.AliasNode) ast.Node {
	if alias.Value == nil || alias.Value.GetToken() == nil {
		return nil
	}
	doc, ok := v.aliasDocs[alias]
	if !ok {
		doc = 1 << 30 // an alias the view has not seen: any document
	}
	var same, before ast.Node
	for _, d := range v.anchors[alias.Value.GetToken().Value] {
		switch {
		case d.doc == doc:
			same = d.node
		case d.doc < doc:
			before = d.node
		}
	}
	if same != nil {
		return same
	}
	return before
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
	node = v.unwrap(node)
	if entries, ok := v.merged[node]; ok {
		return entries
	}
	var values []*ast.MappingValueNode
	switch n := node.(type) {
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
		sources := []ast.Node{mv.Value}
		if seq, ok := v.unwrap(mv.Value).(*ast.SequenceNode); ok {
			sources = seq.Values
		}
		for _, source := range sources {
			if len(merged) >= maxMergedEntries {
				break // merges of merges of the same mappings
			}
			merged = append(merged, v.entriesDepth(source, depth+1)...)
		}
	}
	if len(merged) > maxMergedEntries {
		merged = merged[:maxMergedEntries]
	}
	// The entries of a mapping are computed once: nested merges would
	// otherwise expand the same mappings again and again.
	entries := slices.Concat(own, merged)
	v.merged[node] = entries
	return entries
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
