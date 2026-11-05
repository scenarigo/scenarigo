package lsp

import (
	"strings"

	"github.com/goccy/go-yaml/ast"
)

// yttFillsValue reports whether ytt provides the value of mv. In a ytt
// template, "title: #@ params.title" is a key with a null value followed by
// an annotation comment that ytt evaluates into the value, so the null is not
// what scenarigo will read.
func yttFillsValue(text string, mv *ast.MappingValueNode) bool {
	if _, ok := mv.Value.(*ast.NullNode); !ok {
		return false
	}
	tok := mv.Key.GetToken()
	if tok == nil {
		return false
	}
	line := lineAt(text, tok.Position.Line-1)
	start := byteColumnFromRune(line, tok.Position.Column-1)
	rest, ok := strings.CutPrefix(strings.TrimLeft(line[start:], `"'`), tok.Value)
	if !ok {
		return false // an unusual key, such as one with escapes: keep checking
	}
	rest = strings.TrimLeft(rest, `"'`)
	rest, ok = strings.CutPrefix(strings.TrimLeft(rest, " \t"), ":")
	if !ok {
		return false
	}
	return strings.HasPrefix(strings.TrimLeft(rest, " \t"), "#@")
}
