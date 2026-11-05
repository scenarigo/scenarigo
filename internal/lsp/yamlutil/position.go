package yamlutil

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

// Document holds a parsed YAML document and provides position-based lookups.
type Document struct {
	File *ast.File
	Text string
}

// Limits on an estimate of what a text costs the parser, taken from its tokens
// because running out of memory or stack cannot be recovered from.
const (
	// maxPathBytes bounds the estimated sum of the path lengths of the tokens.
	// The parser builds the path from the root for each value, so what it
	// allocates depends on the keys and the depth above the values, not on the
	// size of the text:
	// 80 KB of "[" took over 8 GiB, and 512 KiB of which one key is half over 4.
	maxPathBytes = 256 << 20
	// maxChain bounds the tags and anchors of one value. The parser recurses on
	// them, and running out of stack ends the process.
	maxChain = 128
	// maxDocuments bounds the document markers of a text. The parser recurses on
	// them too: over 2 million, 10 MiB of "...", ended the process.
	maxDocuments = 100_000
	// indexBytes is what a sequence adds to a path, such as "[1000]".
	indexBytes = 6
)

// Parse parses YAML text into a Document.
// If the text does not parse or exceeds the bounds, it returns nil.
func Parse(text string) *Document {
	tokens := lexer.Tokenize(text)
	if tooCostly(tokens) {
		return nil
	}
	file := parseTokens(tokens)
	if file == nil {
		return nil
	}
	return &Document{
		File: file,
		Text: text,
	}
}

// parseTokens returns the parsed tokens, or nil if they do not parse. That
// includes a panic in go-yaml, as on a tag that ends its document under a
// %TAG !! directive: the text comes from an editor, and the server has to
// outlive it.
func parseTokens(tokens token.Tokens) *ast.File {
	defer func() { _ = recover() }()
	file, err := parser.Parse(tokens, parser.ParseComments)
	if err != nil {
		return nil
	}
	return file
}

// pathLevel is a key, a sequence entry or a flow collection above a token.
type pathLevel struct {
	column int
	bytes  int // of the path up to and including this level
	entry  bool
	flow   bool
}

// pathStack estimates the path of a token: the levels above it, by indentation
// outside flow collections and under a tag that holds a value. A key's level starts at the "?", tag or anchor
// before it: what nests under the key can sit left of the key itself.
type pathStack []pathLevel

func (s pathStack) bytes() int {
	if len(s) == 0 {
		return 1
	}
	return s[len(s)-1].bytes
}

func (s *pathStack) enter(level pathLevel, size int) {
	level.bytes = s.bytes() + size
	*s = append(*s, level)
}

// leave drops the block levels that end for ends, within the innermost flow
// collection.
func (s *pathStack) leave(ends func(pathLevel) bool) {
	for len(*s) > 0 {
		top := (*s)[len(*s)-1]
		if top.flow || !ends(top) {
			return
		}
		*s = (*s)[:len(*s)-1]
	}
}

// closeFlow drops the levels up to and including the innermost flow collection.
func (s *pathStack) closeFlow() {
	for len(*s) > 0 {
		top := (*s)[len(*s)-1]
		*s = (*s)[:len(*s)-1]
		if top.flow {
			return
		}
	}
}

// enterEntry enters a sequence entry. held is true when it is the value of a
// tag, so it nests instead of following the entry at its column.
func (s *pathStack) enterEntry(column int, held bool) {
	if !held {
		// An entry can sit at the column of the key it belongs to.
		s.leave(func(l pathLevel) bool { return l.column > column || l.column == column && l.entry })
	}
	s.enter(pathLevel{column: column, entry: true}, indexBytes)
}

// enterKey enters a mapping key. held is as for enterEntry.
func (s *pathStack) enterKey(column, size int, held bool) {
	if !held {
		s.leave(func(l pathLevel) bool { return l.column >= column })
	}
	s.enter(pathLevel{column: column}, size)
}

// startsValue reports whether a block value comes after a token of type t.
func startsValue(t token.Type) bool {
	return t == token.MappingValueType || t == token.SequenceEntryType
}

// isProperty reports whether a token of type t is a tag or an anchor, which
// stack on one value.
func isProperty(t token.Type) bool {
	return t == token.TagType || t == token.AnchorType
}

// nextToken returns the token after i that is not a comment, or nil.
func nextToken(tokens token.Tokens, i int) *token.Token {
	for _, tk := range tokens[i+1:] {
		if tk.Type != token.CommentType {
			return tk
		}
	}
	return nil
}

// documentMarkers counts the markers in every position: the parser groups
// tokens before it recurses, so what looks like the name of an anchor can be one.
func documentMarkers(tokens token.Tokens) int {
	n := 0
	for _, tk := range tokens {
		if tk.Type == token.DocumentHeaderType || tk.Type == token.DocumentEndType {
			n++
		}
	}
	return n
}

// tooCostly reports whether tokens exceed maxPathBytes, maxChain or maxDocuments.
// It stops what an editor meets by accident, not a text made to pass it: the
// parser also nests in shapes this does not follow, such as at the column of a
// sibling.
func tooCostly(tokens token.Tokens) bool {
	var path pathStack
	if documentMarkers(tokens) > maxDocuments {
		return true
	}
	cost, chain, head := 0, 0, 0
	// held is true while a tag stands where a block value goes. The parser makes
	// what follows its value, even at the column of a sibling on a later line,
	// except a key after !!seq or !!omap, which this overestimates. An anchor
	// there does so only for what is not such a sibling, which this does not
	// follow.
	held := false
	var prev token.Type
	for i := 0; i < len(tokens); i++ {
		tk := tokens[i]
		if tk.Type == token.CommentType {
			continue
		}
		column := tk.Position.Column
		if head != 0 {
			column = min(column, head)
		}
		switch tk.Type {
		case token.TagType:
			chain++
			head = column
			held = held || startsValue(prev)
		case token.AnchorType:
			chain++
			head = column
			i++ // its name
		case token.MappingKeyType:
			chain = 0
			head = column
		case token.DocumentHeaderType, token.DocumentEndType:
			path, chain = path[:0], 0
		case token.SequenceStartType, token.MappingStartType:
			path.enter(pathLevel{flow: true}, indexBytes)
			chain = 0
		case token.SequenceEndType, token.MappingEndType:
			path.closeFlow()
			chain = 0
		case token.CollectEntryType:
			path.leave(func(pathLevel) bool { return true })
			chain = 0
		case token.SequenceEntryType:
			path.enterEntry(column, held)
			chain = 0
		default:
			chain = 0
			if next := nextToken(tokens, i); next != nil && next.Type == token.MappingValueType {
				path.enterKey(column, len(tk.Value)+1, held)
			}
		}
		held = held && isProperty(tk.Type)
		if !isProperty(tk.Type) && tk.Type != token.MappingKeyType {
			head = 0
		}
		if tk.Type != token.AnchorType {
			prev = tk.Type
		}
		cost += path.bytes()
		if cost > maxPathBytes || chain > maxChain {
			return true
		}
	}
	return false
}

// NodePath represents the path from root to a specific node in the YAML AST.
type NodePath struct {
	Keys []string // e.g., ["steps", "0", "request", "method"]
	Node ast.Node
}

// FindNodeAtPosition finds the YAML node at the given position (1-based line, 1-based column).
func (d *Document) FindNodeAtPosition(line, col int) *NodePath {
	if d == nil || d.File == nil {
		return nil
	}
	// Prefer the node whose token spans the column, so that several keys on
	// one line (flow mappings) resolve individually; fall back to the first
	// node on the line so that a cursor in trailing space still hits the key.
	for _, lenient := range []bool{false, true} {
		for _, doc := range d.File.Docs {
			if doc.Body == nil {
				continue
			}
			path := &NodePath{}
			if findNode(doc.Body, line, col, path, lenient) {
				return path
			}
		}
	}
	return nil
}

// tokenSpans reports whether the token covers the column on the line.
// Columns are 1-based rune indexes, as reported by goccy/go-yaml.
func tokenSpans(tok *token.Token, line, col int) bool {
	if tok == nil || tok.Position.Line != line {
		return false
	}
	start := tok.Position.Column
	return col >= start && col < start+utf8.RuneCountInString(tok.Value)
}

func findNode(node ast.Node, line, col int, path *NodePath, lenient bool) bool {
	if node == nil {
		return false
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		for _, v := range n.Values {
			if findNode(v, line, col, path, lenient) {
				return true
			}
		}
	case *ast.MappingValueNode:
		keyTok := n.Key.GetToken()
		onLine := keyTok != nil && keyTok.Position.Line == line
		if tokenSpans(keyTok, line, col) || (lenient && onLine) {
			path.Keys = append(path.Keys, n.Key.String())
			path.Node = n
			return true
		}
		if n.Value != nil {
			valPath := &NodePath{}
			if findNode(n.Value, line, col, valPath, lenient) {
				path.Keys = append(path.Keys, n.Key.String())
				path.Keys = append(path.Keys, valPath.Keys...)
				path.Node = valPath.Node
				return true
			}
		}
	case *ast.SequenceNode:
		for i, v := range n.Values {
			valPath := &NodePath{}
			if findNode(v, line, col, valPath, lenient) {
				path.Keys = append(path.Keys, fmt.Sprintf("%d", i))
				path.Keys = append(path.Keys, valPath.Keys...)
				path.Node = valPath.Node
				return true
			}
		}
	default:
		tok := node.GetToken()
		if tokenSpans(tok, line, col) || (lenient && tok != nil && tok.Position.Line == line) {
			path.Node = node
			return true
		}
	}

	return false
}

// CursorContext describes what the cursor is positioned on.
type CursorContext struct {
	// Type indicates what kind of completion is expected.
	Type CursorContextType
	// Path is the YAML key path to the current position (e.g., ["steps", "0", "request"]).
	Path []string
	// PartialKey is the partial key being typed (for key completion).
	PartialKey string
	// PartialValue is the partial value being typed (for value completion).
	PartialValue string
	// ParentKeys lists sibling keys already present at the same level.
	ParentKeys []string
	// SiblingValues maps sibling key names to their values at the same level.
	// Used for dynamic schema resolution (e.g., protocol value determines request fields).
	SiblingValues map[string]string
}

type CursorContextType int

const (
	CursorContextUnknown CursorContextType = iota
	CursorContextKey                       // Cursor is at a position where a key is expected
	CursorContextValue                     // Cursor is at a position where a value is expected
)

// GetCursorContext analyzes the cursor position and returns context for completion.
func (d *Document) GetCursorContext(line, col int) *CursorContext {
	if d == nil {
		return &CursorContext{Type: CursorContextUnknown}
	}

	// Get the current line text.
	lines := strings.Split(d.Text, "\n")
	if line >= len(lines) {
		return &CursorContext{Type: CursorContextUnknown}
	}
	currentLine := lines[line]

	// Determine if cursor is on key or value side.
	colonIdx := strings.Index(currentLine, ":")
	trimmed := strings.TrimSpace(currentLine)

	// Helper to build key context with sibling values.
	keyContext := func(partialKey string) *CursorContext {
		siblings := make(map[string]string)
		path, parentKeys := d.getPathAndSiblingsForIndent(line, siblings)
		return &CursorContext{
			Type:          CursorContextKey,
			Path:          path,
			PartialKey:    partialKey,
			ParentKeys:    parentKeys,
			SiblingValues: siblings,
		}
	}

	// Empty line or only whitespace: key completion at current indent level.
	if trimmed == "" {
		return keyContext("")
	}

	// Line starts with "- " or is just "-": sequence item.
	if trimmed == "-" {
		return keyContext("")
	}
	if after, ok := strings.CutPrefix(trimmed, "- "); ok {
		afterDash := after
		found := strings.Contains(afterDash, ":")
		if !found {
			return keyContext(strings.TrimSpace(afterDash))
		}
	}

	if colonIdx < 0 {
		return keyContext(trimmed)
	}

	if col <= colonIdx {
		return keyContext(strings.TrimSpace(currentLine[:col]))
	}

	// Cursor is after colon: value context.
	// Build path using text-based analysis: parent path + current key.
	siblings := make(map[string]string)
	parentPath, _ := d.getPathAndSiblingsForIndent(line, siblings)
	currentKey := extractKey(trimmed)
	var path []string
	path = append(path, parentPath...)
	if currentKey != "" {
		path = append(path, currentKey)
	}
	valueText := strings.TrimSpace(currentLine[colonIdx+1:])
	return &CursorContext{
		Type:          CursorContextValue,
		Path:          path,
		PartialValue:  valueText,
		SiblingValues: siblings,
	}
}

// getPathAndSiblingsForIndent determines the schema path for a given indent level
// by walking backwards through lines to find parent and sibling keys.
//
// YAML structure awareness:
//   - Sequence items ("- key: val") have their content indent at (line_indent + 2).
//   - When we encounter a "- " line, its keys are siblings if content indent matches.
//   - A line with lower indent that ends with ":" (no value) is a parent mapping.
//
// If siblingValues is non-nil, it also collects key=value pairs at the same indent level.
func (d *Document) getPathAndSiblingsForIndent(line int, siblingValues map[string]string) ([]string, []string) {
	var path, parentKeys []string
	lines := strings.Split(d.Text, "\n")
	if line >= len(lines) {
		return nil, nil
	}

	currentIndent := effectiveIndent(lines[line])

	// Walk backwards to find parent keys.
	for i := line - 1; i >= 0; i-- {
		lineText := lines[i]
		trimmed := strings.TrimSpace(lineText)
		if trimmed == "" {
			continue
		}

		ei := effectiveIndent(lineText)

		// Same effective indent: sibling key.
		if ei == currentIndent {
			if key := extractKey(trimmed); key != "" {
				parentKeys = append(parentKeys, key)
				if siblingValues != nil {
					if val := extractValue(trimmed); val != "" {
						siblingValues[key] = val
					}
				}
			}
		}

		// Lower effective indent: parent key.
		if ei < currentIndent {
			key := extractKey(trimmed)
			if key != "" {
				path = append([]string{key}, path...)
				currentIndent = ei
			}
		}
	}

	return path, parentKeys
}

// effectiveIndent returns the indent level where mapping keys actually live.
// For "  - key: val", the raw indent is 2 but the effective key indent is 4
// (because "- " shifts content by 2).
func effectiveIndent(line string) int {
	raw := getIndent(line)
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- ") {
		return raw + 2
	}
	return raw
}

func getIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func extractKey(trimmedLine string) string {
	// Handle "- key: value" (sequence item with mapping).
	s := trimmedLine
	if after, ok := strings.CutPrefix(s, "- "); ok {
		s = after
	}
	before, _, ok := strings.Cut(s, ":")
	if !ok {
		return ""
	}
	return strings.TrimSpace(before)
}

func extractValue(trimmedLine string) string {
	s := trimmedLine
	if after, ok := strings.CutPrefix(s, "- "); ok {
		s = after
	}
	_, after, ok := strings.Cut(s, ":")
	if !ok {
		return ""
	}
	val := strings.TrimSpace(after)
	// Strip quotes.
	if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
		val = val[1 : len(val)-1]
	}
	return val
}
