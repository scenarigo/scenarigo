package source

import "github.com/goccy/go-yaml/ast"

// File represents a source file of any type.
type File interface {
	Read() ([]byte, error)
	Line(int) (string, error)
	Doc() *ast.Document
}

type file struct{}
