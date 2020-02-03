package parser

import (
	"strconv"
	"strings"

	"github.com/goccy/go-yaml/printer"
	yamltoken "github.com/goccy/go-yaml/token"

	"github.com/zoncoen/scenarigo/template/token"
)

type yamlScanner struct {
	printer printer.Printer
	tokens  yamltoken.Tokens
	pos     int

	child         *scanner
	childPos      int
	childTok      token.Token
	childLit      string
	preChildLit   string
	afterChildPos int
}

func (s *yamlScanner) scan() (int, token.Token, string) {
	if s.child != nil {
		if s.childTok != token.EOF {
			defer s.next()
			return s.childPos, s.childTok, s.childLit
		}
		s.pos = s.afterChildPos
		s.child = nil
	}

	if len(s.tokens) == 0 {
		suf := s.suffix()
		return s.pos - len(suf), token.EOF, suf
	}

	tokens := make([]*yamltoken.Token, 0, len(s.tokens))
L:
	for {
		if len(s.tokens) == 0 {
			break
		}
		var tok *yamltoken.Token
		tok, s.tokens = s.tokens[0], s.tokens[1:]
		switch tok.Type {
		case yamltoken.StringType, yamltoken.SingleQuoteType, yamltoken.DoubleQuoteType:
			str := tok.Origin
			pos := s.pos
			if tok.Type == yamltoken.SingleQuoteType {
				str = strings.Replace(str, "'"+tok.Value+"'", tok.Value, 1)
				pos++
			} else if tok.Type == yamltoken.DoubleQuoteType {
				str = strings.Replace(str, strconv.Quote(tok.Value), tok.Value, 1)
				pos++
			}
			s.child = newScanner(strings.NewReader(str))
			s.child.pos = pos

			s.afterChildPos = s.pos + runesLen(tok.Origin)
			s.next()

			// empty string
			if s.childTok == token.EOF {
				s.child = nil
				tokens = append(tokens, tok)
				s.pos += runesLen(tok.Origin)
				continue
			}

			// no template
			if s.childTok == token.STRING {
				lit := s.childLit
				s.next()
				if s.childTok == token.EOF {
					s.child = nil
					tokens = append(tokens, tok)
					s.pos += runesLen(tok.Origin)
					continue
				}
				tok.Value = lit
				tok.Origin = lit
				tokens = append(tokens, tok)
				s.pos += runesLen(tok.Origin)
			}

			break L
		}

		tokens = append(tokens, tok)
		s.pos += runesLen(tok.Origin)
	}

	str := s.printer.PrintTokens(tokens)
	return s.pos - runesLen(str), token.STRING, str
}

func (s *yamlScanner) next() {
	s.preChildLit = s.childLit
	s.childPos, s.childTok, s.childLit = s.child.scan()
}

func (s *yamlScanner) suffix() string {
	lastLit := []rune(s.preChildLit)
	l := len(lastLit)
	var suffix []rune
L:
	for i := 1; i <= l; i++ {
		switch ch := lastLit[l-i]; ch {
		case ' ':
			suffix = append([]rune{ch}, suffix...)
		case '\n':
			suffix = append([]rune{ch}, suffix...)
			break L
		default:
			break L
		}
	}
	return string(suffix)
}
