// Package parser implements a parser for a template string.
package parser

import (
	"io"

	"github.com/zoncoen/scenarigo/template/ast"
	"github.com/zoncoen/scenarigo/template/token"
)

// Parser represents a parser.
type Parser struct {
	s      *scanner
	cal    *posCalculator
	pos    int
	tok    token.Token
	lit    string
	errors Errors
}

// NewParser returns a new parser.
func NewParser(r io.Reader) *Parser {
	cal := &posCalculator{}
	return &Parser{
		s:   newScanner(io.TeeReader(r, cal)),
		cal: cal,
	}
}

// Parse parses the template string and returns the corresponding ast.Node.
func (p *Parser) Parse() (ast.Node, error) {
	p.next()
	if p.tok == token.EOF {
		// empty string
		return &ast.BasicLit{
			ValuePos: 0,
			Kind:     token.STRING,
			Value:    "",
		}, nil
	}
	return p.parseExpr(), p.errors.Err()
}

func (p *Parser) next() {
	p.pos, p.tok, p.lit = p.s.scan()
}

func (p *Parser) parseExpr() ast.Expr {
	return p.parseBinaryExpr(token.LowestPrec + 1)
}

func (p *Parser) parseBinaryExpr(prec int) ast.Expr {
	x := p.parseOperand()
L:
	for {
		if p.tok == token.LINE_BREAK {
			return x
		}

		oprec := p.tok.Precedence()
		if oprec < prec {
			return x
		}

		switch p.tok {
		case token.ADD:
			pos := p.pos
			p.next()
			y := p.parseBinaryExpr(oprec + 1)
			x = &ast.BinaryExpr{
				X:     x,
				OpPos: pos,
				Op:    token.ADD,
				Y:     y,
			}
		case token.CALL:
			pos := p.pos
			p.next()
			args := make([]ast.Expr, 0, 1)
			if y := p.parseExpr(); y != nil {
				args = append(args, y)
			}
			x = &ast.CallExpr{
				Fun:    x,
				Lparen: pos,
				Args:   args,
				Rparen: pos,
			}
		case token.LARROW:
			pos := p.pos
			p.next()
			x = &ast.LeftArrowExpr{
				Fun:     x,
				Larrow:  pos,
				Rdbrace: p.expect(token.RDBRACE),
				Arg:     p.parseExpr(),
			}
			if p.tok == token.LINE_BREAK {
				if p.lit != "" {
					p.tok = token.STRING
					return x
				}
				p.next()
				return x
			}
		case token.LDBRACE, token.STRING:
			pos := p.pos
			y := p.parseBinaryExpr(oprec + 1)
			x = &ast.BinaryExpr{
				X:     x,
				OpPos: pos,
				Op:    token.ADD,
				Y:     y,
			}
		default:
			break L
		}
	}
	return x
}

func (p *Parser) parseIdent() *ast.Ident {
	pos := p.pos
	name := "_"
	if p.tok == token.IDENT {
		name = p.lit
		p.next()
	} else {
		p.expect(token.IDENT)
	}
	return &ast.Ident{NamePos: pos, Name: name}
}

func (p *Parser) parseOperand() ast.Expr {
	var e ast.Expr
	switch p.tok {
	case token.STRING, token.INT:
		e = &ast.BasicLit{
			ValuePos: p.pos,
			Kind:     p.tok,
			Value:    p.lit,
		}
		p.next()
	case token.IDENT:
		e = p.parseIdent()
	L:
		for {
			switch p.tok {
			case token.PERIOD:
				p.next()
				e = &ast.SelectorExpr{
					X:   e,
					Sel: p.parseIdent(),
				}
			case token.LBRACK:
				lbrack := p.pos
				p.next()
				index := p.parseExpr()
				e = &ast.IndexExpr{
					X:      e,
					Lbrack: lbrack,
					Index:  index,
					Rbrack: p.expect(token.RBRACK),
				}
			case token.LPAREN:
				lparen := p.pos
				p.next()
				e = &ast.CallExpr{
					Fun:    e,
					Lparen: lparen,
					Args:   p.parseArgs(),
					Rparen: p.expect(token.RPAREN),
				}
			default:
				break L
			}
		}
	case token.LDBRACE:
		e = p.parseParameter()
	default:
		return nil
	}
	return e
}

func (p *Parser) parseParameter() ast.Expr {
	param := &ast.ParameterExpr{
		Ldbrace: p.pos,
	}
	p.next()
	param.X = p.parseExpr()

	if lae, ok := param.X.(*ast.LeftArrowExpr); ok {
		param.Rdbrace = lae.Rdbrace
		return param
	}
	param.Rdbrace = p.expect(token.RDBRACE)
	return param
}

func (p *Parser) parseArgs() []ast.Expr {
	args := []ast.Expr{}
	if p.tok == token.RPAREN {
		return args
	}
	args = append(args, p.parseExpr())
	for p.tok == token.COMMA {
		p.next()
		args = append(args, p.parseExpr())
	}
	return args
}

func (p *Parser) error(pos int, msg string) {
	p.errors.Append(pos, msg)
}

func (p *Parser) errorExpected(pos int, msg string) {
	msg = "expected " + msg
	if pos == p.pos {
		msg += ", found '" + p.tok.String() + "'"
	}
	p.error(pos, msg)
}

func (p *Parser) expect(tok token.Token) int {
	pos := p.pos
	if p.tok != tok {
		p.errorExpected(pos, "'"+tok.String()+"'")
	}
	p.next() // make progress
	return pos
}

// Pos returns the Position value for the given offset.
func (p *Parser) Pos(pos int) *Position {
	return p.cal.Pos(pos)
}
