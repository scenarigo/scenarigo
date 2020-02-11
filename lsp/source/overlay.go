package source

import (
	"github.com/go-language-server/uri"
	"github.com/goccy/go-yaml/ast"
)

type overlay struct {
	uri   uri.URI
	data  []byte
	lines []string
	doc   *ast.Document

	// unchanged is true if a file has not yet been edited.
	unchanged bool
}

func (o *overlay) Read() ([]byte, error) {
	return o.data, nil
}

func (o *overlay) Line(n int) (string, error) {
	return o.lines[n], nil
}

func (o *overlay) Doc() *ast.Document {
	return o.doc
}
