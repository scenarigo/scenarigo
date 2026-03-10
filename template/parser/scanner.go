package parser

import (
	"bufio"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	yamltoken "github.com/goccy/go-yaml/token"

	"github.com/scenarigo/scenarigo/template/token"
)

// eof represents invalid code points.
var eof = unicode.ReplacementChar

type scanner struct {
	r                  *bufio.Reader
	pos                int
	buf                []rune
	isReadingParameter bool

	// for left arrow expression
	expectColon bool
	yamlScanner *yamlScanner
	indicator   yamltoken.Indicator
}

func newScanner(r io.Reader) *scanner {
	return &scanner{
		r:   bufio.NewReader(r),
		pos: 1,
	}
}

func (s *scanner) read() rune {
	if len(s.buf) > 0 {
		var ch rune
		ch, s.buf = s.buf[0], s.buf[1:]
		s.pos++
		return ch
	}
	ch, _, err := s.r.ReadRune()
	if err != nil {
		return eof
	}
	s.pos++
	return ch
}

func (s *scanner) unread(ch rune) {
	if ch == eof {
		return
	}
	s.buf = append(s.buf, ch)
	s.pos--
}

func (s *scanner) skipSpaces() {
	for {
		if ch := s.read(); ch != ' ' && ch != '\n' && ch != '\r' {
			s.unread(ch)
			return
		}
	}
}

func (s *scanner) scanRawString() (int, token.Token, string) {
	pos := s.pos
	var b strings.Builder
scan:
	for {
		switch ch := s.read(); ch {
		case eof:
			if b.Len() == 0 {
				return pos, token.EOF, ""
			}
			break scan
		case '\\':
			switch next := s.read(); next {
			case '\\', '{':
				b.WriteRune(next)
			default:
				b.WriteRune(ch)
				s.unread(next)
			}
		case '{':
			next := s.read()
			if next == '{' {
				if b.Len() == 0 {
					return pos, token.LDBRACE, "{{"
				}
				s.unread(ch)
				s.unread(next)
				break scan
			}
			s.unread(next)
			b.WriteRune(ch)
		default:
			b.WriteRune(ch)
		}
	}
	str := b.String()
	return pos, token.STRING, str
}

func (s *scanner) scanString() (int, token.Token, string) {
	var b strings.Builder
scan:
	for {
		ch := s.read()
		switch ch {
		case eof:
			// string not terminated
			return s.pos, token.ILLEGAL, ""
		case '"':
			break scan
		default:
			b.WriteRune(ch)
		}
	}
	str := b.String()
	return s.pos - runesLen(str) - 2, token.STRING, str
}

func (s *scanner) scanInt(head rune) (int, token.Token, string) {
	var b strings.Builder
	b.WriteRune(head)
	tk := token.INT
scan:
	for {
		ch := s.read()
		if !isDigit(ch) {
			if ch != '.' {
				s.unread(ch)
				break scan
			}
			if tk == token.INT {
				tk = token.FLOAT
			} else {
				tk = token.ILLEGAL
			}
		}
		b.WriteRune(ch)
	}
	switch tk {
	case token.INT:
		if head == '0' && b.Len() != 1 {
			return s.pos - b.Len(), token.ILLEGAL, b.String()
		}
	case token.FLOAT:
		runes := []rune(b.String())
		if head == '0' && runes[1] != '.' {
			return s.pos - b.Len(), token.ILLEGAL, b.String()
		}
		if runes[len(runes)-1] == '.' {
			return s.pos - b.Len(), token.ILLEGAL, b.String()
		}
	}
	return s.pos - b.Len(), tk, b.String()
}

func (s *scanner) scanIdent(head rune) (int, token.Token, string) {
	var b strings.Builder
	b.WriteRune(head)
scan:
	for {
		ch := s.read()
		switch ch {
		case '-', '_':
			b.WriteRune(ch)
			continue
		default:
			if isLetter(ch) || isDigit(ch) {
				b.WriteRune(ch)
				continue
			}
		}
		s.unread(ch)
		break scan
	}
	str := b.String()
	switch str {
	case "true", "false":
		return s.pos - runesLen(str), token.BOOL, str
	case "nil", "null":
		return s.pos - runesLen(str), token.NIL, str
	case "defined":
		return s.pos - runesLen(str), token.DEFINED, str
	}
	return s.pos - runesLen(str), token.IDENT, str
}

func (s *scanner) scan() (int, token.Token, string) {
	if s.yamlScanner != nil {
		pos, tok, lit := s.yamlScanner.scan()
		if tok == token.EOF {
			s.yamlScanner = nil
			return pos, token.LINEBREAK, lit
		}
		return pos, tok, lit
	}

	if !s.isReadingParameter {
		if s.expectColon {
			s.expectColon = false
			switch ch := s.read(); ch {
			case ':':
				b, err := io.ReadAll(s.r)
				if err != nil {
					return s.pos, token.ILLEGAL, err.Error()
				}
				s.yamlScanner = newYAMLScanner(string(b), s.pos)
				return s.scan()
			case eof:
				return s.pos, token.EOF, ""
			default:
				return s.pos - 1, token.ILLEGAL, string(ch)
			}
		}

		pos, tok, lit := s.scanRawString()
		if tok == token.LDBRACE {
			s.isReadingParameter = true
		}
		return pos, tok, lit
	}

	return s.scanToken()
}

//nolint:cyclop,gocyclo
func (s *scanner) scanToken() (int, token.Token, string) {
	s.skipSpaces()
	ch := s.read()
	switch ch {
	case eof:
		return s.pos, token.EOF, ""
	case '(':
		return s.pos - 1, token.LPAREN, "("
	case ')':
		return s.pos - 1, token.RPAREN, ")"
	case '[':
		return s.pos - 1, token.LBRACK, "["
	case ']':
		return s.pos - 1, token.RBRACK, "]"
	case '}':
		next := s.read()
		if next == '}' {
			s.isReadingParameter = false
			return s.pos - 2, token.RDBRACE, "}}"
		}
		s.unread(next)
	case ',':
		return s.pos - 1, token.COMMA, ","
	case '.':
		return s.pos - 1, token.PERIOD, "."
	case '?':
		next := s.read()
		if next == '?' {
			return s.pos - 2, token.COALESCING, "??"
		}
		s.unread(next)
		return s.pos - 1, token.QUESTION, "?"
	case ':':
		return s.pos - 1, token.COLON, ":"
	case '+':
		return s.pos - 1, token.ADD, "+"
	case '-':
		return s.pos - 1, token.SUB, "-"
	case '*':
		return s.pos - 1, token.MUL, "*"
	case '/':
		return s.pos - 1, token.QUO, "/"
	case '%':
		return s.pos - 1, token.REM, "%"
	case '&':
		next := s.read()
		if next == '&' {
			return s.pos - 2, token.LAND, "&&"
		}
		s.unread(next)
	case '|':
		next := s.read()
		if next == '|' {
			return s.pos - 2, token.LOR, "||"
		}
		s.unread(next)
	case '=':
		next := s.read()
		if next == '=' {
			return s.pos - 2, token.EQL, "=="
		}
		s.unread(next)
	case '!':
		next := s.read()
		if next == '=' {
			return s.pos - 2, token.NEQ, "!="
		}
		s.unread(next)
		return s.pos - 1, token.NOT, "!"
	case '<':
		next := s.read()
		switch next {
		case '=':
			return s.pos - 2, token.LEQ, "<="
		case '-':
			s.expectColon = true
			return s.pos - 2, token.LARROW, "<-"
		}
		s.unread(next)
		return s.pos - 1, token.LSS, "<"
	case '>':
		next := s.read()
		if next == '=' {
			return s.pos - 2, token.GEQ, ">="
		}
		s.unread(next)
		return s.pos - 1, token.GTR, ">"
	case '$':
		return s.pos - 1, token.IDENT, string(ch)
	default:
		if ch == '"' {
			return s.scanString()
		}
		if isDigit(ch) {
			return s.scanInt(ch)
		}
		if isLetter(ch) {
			return s.scanIdent(ch)
		}
	}
	return s.pos - 1, token.ILLEGAL, string(ch)
}

func (s *scanner) quoted() bool {
	if s.yamlScanner != nil {
		return s.yamlScanner.quoted()
	}
	return s.indicator == yamltoken.QuotedScalarIndicator
}

func isLetter(ch rune) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch >= utf8.RuneSelf && unicode.IsLetter(ch)
}

func isDigit(ch rune) bool {
	return '0' <= ch && ch <= '9' || ch >= utf8.RuneSelf && unicode.IsDigit(ch)
}

func runesLen(s string) int {
	return len([]rune(s))
}
