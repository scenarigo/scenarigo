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
	lines, l := linesOf(text), tok.Position.Line-1
	line := lines.text(text, l)
	start := lines.byteColumn(text, l, tok.Position.Column-1)
	rest, ok := strings.CutPrefix(line[start:], tok.Value)
	if !ok {
		return false // quoted or otherwise unusual key: keep checking
	}
	rest, ok = strings.CutPrefix(strings.TrimLeft(rest, " \t"), ":")
	if !ok {
		return false
	}
	return strings.HasPrefix(strings.TrimLeft(rest, " \t"), "#@")
}
