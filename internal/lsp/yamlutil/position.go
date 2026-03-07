package yamlutil

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// Document holds a parsed YAML document and provides position-based lookups.
type Document struct {
	File *ast.File
	Text string
}

// Parse parses YAML text into a Document.
// If parsing fails, it returns nil (caller should use cached document).
func Parse(text string) *Document {
	file, err := parser.ParseBytes([]byte(text), parser.ParseComments)
	if err != nil {
		// Try to parse anyway - go-yaml sometimes returns both file and error.
		if file == nil || len(file.Docs) == 0 {
			return nil
		}
	}
	return &Document{
		File: file,
		Text: text,
	}
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
	for _, doc := range d.File.Docs {
		if doc.Body == nil {
			continue
		}
		path := &NodePath{}
		if findNode(doc.Body, line, col, path) {
			return path
		}
	}
	return nil
}

func findNode(node ast.Node, line, col int, path *NodePath) bool {
	if node == nil {
		return false
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		for _, v := range n.Values {
			if findNode(v, line, col, path) {
				return true
			}
		}
	case *ast.MappingValueNode:
		keyTok := n.Key.GetToken()
		if keyTok != nil && keyTok.Position.Line == line {
			path.Keys = append(path.Keys, n.Key.String())
			path.Node = n
			return true
		}
		if n.Value != nil {
			valPath := &NodePath{}
			if findNode(n.Value, line, col, valPath) {
				path.Keys = append(path.Keys, n.Key.String())
				path.Keys = append(path.Keys, valPath.Keys...)
				path.Node = valPath.Node
				return true
			}
		}
		// Check if cursor is on value position (same line as key, after colon)
		if keyTok != nil && keyTok.Position.Line == line {
			path.Keys = append(path.Keys, n.Key.String())
			path.Node = n
			return true
		}
	case *ast.SequenceNode:
		for i, v := range n.Values {
			valPath := &NodePath{}
			if findNode(v, line, col, valPath) {
				path.Keys = append(path.Keys, fmt.Sprintf("%d", i))
				path.Keys = append(path.Keys, valPath.Keys...)
				path.Node = valPath.Node
				return true
			}
		}
	default:
		tok := node.GetToken()
		if tok != nil && tok.Position.Line == line {
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
	CursorContextKey                        // Cursor is at a position where a key is expected
	CursorContextValue                      // Cursor is at a position where a value is expected
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

	// Line starts with "- ": sequence item, could be key completion.
	if strings.HasPrefix(trimmed, "- ") {
		afterDash := strings.TrimPrefix(trimmed, "- ")
		subColonIdx := strings.Index(afterDash, ":")
		if subColonIdx < 0 {
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
func (d *Document) getPathAndSiblingsForIndent(line int, siblingValues map[string]string) (path []string, parentKeys []string) {
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
	if strings.HasPrefix(s, "- ") {
		s = strings.TrimPrefix(s, "- ")
	}
	colonIdx := strings.Index(s, ":")
	if colonIdx < 0 {
		return ""
	}
	return strings.TrimSpace(s[:colonIdx])
}

func extractValue(trimmedLine string) string {
	s := trimmedLine
	if strings.HasPrefix(s, "- ") {
		s = strings.TrimPrefix(s, "- ")
	}
	colonIdx := strings.Index(s, ":")
	if colonIdx < 0 {
		return ""
	}
	val := strings.TrimSpace(s[colonIdx+1:])
	// Strip quotes.
	if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
		val = val[1 : len(val)-1]
	}
	return val
}
