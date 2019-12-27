package completor

import (
	"github.com/goccy/go-yaml/ast"
)

// A cursor describes a node and its parent node.
type cursor struct {
	node   ast.Node
	parent *cursor
}

func (c *cursor) setParent(parent *cursor) {
	if c.parent == nil {
		c.parent = parent
		return
	}
	c.parent.setParent(parent)
}

// A finder finds the cursor by position.
type finder struct {
	line   int
	column int
}

func (f *finder) find(node ast.Node) *cursor {
	if node.Type() != ast.DocumentType {
		pos := node.GetToken().Position
		if pos.Line == f.line {
			// empty node
			if pos.Column == pos.Offset && node.Type() == ast.NullType {
				if pos.Column <= f.column {
					return &cursor{
						node: node,
					}
				}
			}
			if pos.Column <= f.column && f.column <= pos.Offset {
				return &cursor{
					node: node,
				}
			}
		}
	}

	switch n := node.(type) {
	case *ast.NullNode:
	case *ast.IntegerNode:
	case *ast.FloatNode:
	case *ast.StringNode:
	case *ast.MergeKeyNode:
	case *ast.BoolNode:
	case *ast.InfinityNode:
	case *ast.NanNode:
	case *ast.MappingNode:
		for _, v := range n.Values {
			if c := f.find(v); c != nil {
				c.setParent(&cursor{
					node: node,
				})
				return c
			}
		}
	case *ast.MappingValueNode:
		if c := f.find(n.Key); c != nil {
			c.setParent(&cursor{
				node: node,
			})
			return c
		}
		if c := f.find(n.Value); c != nil {
			c.setParent(&cursor{
				node: node,
			})
			return c
		}
	case *ast.SequenceNode:
		for _, value := range n.Values {
			if c := f.find(value); c != nil {
				c.setParent(&cursor{
					node: node,
				})
				return c
			}
		}
	case *ast.AnchorNode:
		if c := f.find(n.Name); c != nil {
			c.setParent(&cursor{
				node: node,
			})
			return c
		}
		if c := f.find(n.Value); c != nil {
			c.setParent(&cursor{
				node: node,
			})
			return c
		}
	case *ast.AliasNode:
		if c := f.find(n.Value); c != nil {
			c.setParent(&cursor{
				node: node,
			})
			return c
		}
	case *ast.Document:
		if c := f.find(n.Body); c != nil {
			c.setParent(&cursor{
				node: node,
			})
			return c
		}
	}

	return nil
}
