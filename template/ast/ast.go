// Package ast declares the types used to represent syntax trees.
package ast

import (
	"github.com/zoncoen/scenarigo/template/token"
)

// All node types implement the Node interface.
type Node interface {
	Pos() int
}

// All expression nodes implement the Expr interface.
type Expr interface {
	Node
	exprNode()
}

type (
	// BadExpr node is a placeholder for expressions containing syntax errors.
	BadExpr struct {
		ValuePos int
		Kind     token.Token
		Value    string
	}

	// BinaryExpr node represents a binary expression.
	BinaryExpr struct {
		X     Expr
		OpPos int
		Op    token.Token
		Y     Expr
	}

	// BasicLit node represents a literal of basic type.
	BasicLit struct {
		ValuePos int
		Kind     token.Token
		Value    string
	}

	// ParameterExpr node represents a parameter of template.
	ParameterExpr struct {
		Ldbrace int
		X       Expr
		Rdbrace int
		Quoted  bool
	}

	// Ident node represents an identifier.
	Ident struct {
		NamePos int
		Name    string
	}

	// SelectorExpr node represents an expression followed by a selector.
	SelectorExpr struct {
		X   Expr
		Sel *Ident
	}

	// IndexExpr node represents an expression followed by an index.
	IndexExpr struct {
		X      Expr
		Lbrack int
		Index  Expr
		Rbrack int
	}

	// A CallExpr node represents an expression followed by an argument list.
	CallExpr struct {
		Fun    Expr
		Lparen int
		Args   []Expr
		Rparen int
	}

	// A LeftArrowExpr node represents an expression followed by an argument.
	LeftArrowExpr struct {
		Fun     Expr
		Larrow  int
		Rdbrace int
		Arg     Expr
	}
)

// Pos implements Node.
func (e *BadExpr) Pos() int       { return e.ValuePos }
func (e *BinaryExpr) Pos() int    { return e.OpPos }
func (e *BasicLit) Pos() int      { return e.ValuePos }
func (e *ParameterExpr) Pos() int { return e.Ldbrace }
func (e *Ident) Pos() int         { return e.NamePos }
func (e *SelectorExpr) Pos() int  { return e.Sel.Pos() }
func (e *IndexExpr) Pos() int     { return e.Lbrack }
func (e *CallExpr) Pos() int      { return e.Lparen }
func (e *LeftArrowExpr) Pos() int { return e.Larrow }

// exprNode implements Expr.
func (e *BadExpr) exprNode()       {}
func (e *BinaryExpr) exprNode()    {}
func (e *BasicLit) exprNode()      {}
func (e *ParameterExpr) exprNode() {}
func (e *Ident) exprNode()         {}
func (e *SelectorExpr) exprNode()  {}
func (e *IndexExpr) exprNode()     {}
func (e *LeftArrowExpr) exprNode() {}
func (e *CallExpr) exprNode()      {}
