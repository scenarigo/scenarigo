package parser

import (
	"strconv"
	"strings"

	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/printer"
	yamltoken "github.com/goccy/go-yaml/token"

	"github.com/scenarigo/scenarigo/template/token"
)

type yamlScanner struct {
	printer printer.Printer
	tokens  yamltoken.Tokens
	pos     int
	trail   string

	child         *scanner
	childPos      int
	childTok      token.Token
	childLit      string
	preChildLit   string
	afterChildPos int
}

func newYAMLScanner(s string, pos int) *yamlScanner {
	// avoid to lost trailling spaces and linebreaks
	runes := []rune(s)
	trail := []rune{}
	for i := len(runes) - 1; i >= 0; i-- {
		switch ch := runes[i]; ch {
		case ' ', '\n':
			trail = append([]rune{ch}, trail...)
			runes = runes[:i]
			continue
		}
		break
	}

	return &yamlScanner{
		tokens: lexer.Tokenize(string(runes)),
		pos:    pos,
		trail:  string(trail),
	}
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
		return s.pos, token.EOF, s.trail
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
			s.child.indicator = tok.Indicator
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

			if s.childTok == token.STRING {
				s.next()
				if s.childTok == token.EOF { // no template
					s.child = nil
					tokens = append(tokens, tok)
					s.pos += runesLen(tok.Origin)
					continue
				}
				tok.Value = s.preChildLit
				tok.Origin = s.preChildLit
				tokens = append(tokens, tok)
				s.pos += runesLen(tok.Origin)
			}

			break L

		default:
			tokens = append(tokens, tok)
			s.pos += runesLen(tok.Origin)
		}
	}

	str := s.printer.PrintTokens(tokens)
	return s.pos - runesLen(str), token.STRING, str
}

func (s *yamlScanner) next() {
	s.preChildLit = s.childLit
	s.childPos, s.childTok, s.childLit = s.child.scan()
}

func (s *yamlScanner) quoted() bool {
	if s.child != nil {
		return s.child.quoted()
	}
	return false
}
