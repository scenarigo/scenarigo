// Package parser implements a parser for a template string.
package parser

import (
	"io"

	"github.com/scenarigo/scenarigo/template/ast"
	"github.com/scenarigo/scenarigo/template/token"
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
	x := p.parseUnaryExpr()
L:
	for {
		if p.tok == token.LINEBREAK {
			return x
		}

		oprec := p.tok.Precedence()
		if oprec < prec {
			return x
		}

		switch p.tok {
		case token.ADD, token.SUB, token.MUL, token.QUO, token.REM,
			token.LAND, token.LOR, token.COALESCING,
			token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			pos := p.pos
			tok := p.tok
			p.next()
			y := p.parseBinaryExpr(oprec + 1)
			x = &ast.BinaryExpr{
				X:     x,
				OpPos: pos,
				Op:    tok,
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
			if p.tok == token.LINEBREAK {
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
				Op:    token.CONCAT,
				Y:     y,
			}
		case token.QUESTION:
			cond := x
			question := p.pos
			p.next()
			expr1 := p.parseExpr()
			x = &ast.ConditionalExpr{
				Condition: cond,
				Question:  question,
				X:         expr1,
				Colon:     p.expect(token.COLON),
				Y:         p.parseExpr(),
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

func (p *Parser) parseUnaryExpr() ast.Expr {
	var e ast.Expr
	switch p.tok {
	case token.STRING, token.INT, token.FLOAT, token.BOOL:
		e = &ast.BasicLit{
			ValuePos: p.pos,
			Kind:     p.tok,
			Value:    p.lit,
		}
		p.next()
	case token.LPAREN:
		pos := p.pos
		p.next()
		e = &ast.ParenExpr{
			Lparen: pos,
			X:      p.parseExpr(),
			Rparen: p.expect(token.RPAREN),
		}
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
	case token.SUB:
		pos := p.pos
		p.next()
		e = &ast.UnaryExpr{
			OpPos: pos,
			Op:    token.SUB,
			X:     p.parseUnaryExpr(),
		}
	case token.NOT:
		pos := p.pos
		p.next()
		e = &ast.UnaryExpr{
			OpPos: pos,
			Op:    token.NOT,
			X:     p.parseUnaryExpr(),
		}
	case token.DEFINED:
		pos := p.pos
		p.next()
		e = &ast.DefinedExpr{
			DefinedPos: pos,
			Lparen:     p.expect(token.LPAREN),
			Arg:        p.parseExpr(),
			Rparen:     p.expect(token.RPAREN),
		}
	default:
		return nil
	}
	return e
}

func (p *Parser) parseParameter() ast.Expr {
	param := &ast.ParameterExpr{
		Ldbrace: p.pos,
		Quoted:  p.s.quoted(),
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
