package parser

import (
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/template/token"
)

func TestScanner_Read(t *testing.T) {
	tests := map[string]struct {
		s         string
		buf       []rune
		expect    rune
		expectPos int
	}{
		"EOF": {
			expect:    eof,
			expectPos: 1,
		},
		"read": {
			s:         "abc",
			expect:    'a',
			expectPos: 2,
		},
		"read from buffer": {
			s:         "abc",
			buf:       []rune{'A', 'B', 'C'},
			expect:    'A',
			expectPos: 2,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			s := newScanner(strings.NewReader(test.s))
			s.buf = test.buf
			if got := s.read(); got != test.expect {
				t.Errorf("expected %q but got %q", test.expect, got)
			}
			if got := s.pos; got != test.expectPos {
				t.Errorf("expected %d but got %d", test.expectPos, got)
			}
		})
	}
}

func TestScanner_Unread(t *testing.T) {
	s := &scanner{
		pos: 4,
	}
	s.unread('a')
	s.unread('b')
	s.unread('c')
	if got, expect := string(s.buf), "abc"; got != expect {
		t.Errorf("expected %q but got %q", expect, got)
	}
	if got, expect := s.pos, 1; got != expect {
		t.Errorf("expected %d but got %d", expect, got)
	}
}

func TestScanner_Scan(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		type result struct {
			pos int
			tok token.Token
			lit string
		}
		tests := map[string]struct {
			src      string
			expected []result
		}{
			"empty": {
				src:      "",
				expected: []result{},
			},
			"no parameter": {
				src: "test",
				expected: []result{
					{
						pos: 1,
						tok: token.STRING,
						lit: "test",
					},
				},
			},
			`escape \`: {
				src: `\\`,
				expected: []result{
					{
						pos: 1,
						tok: token.STRING,
						lit: `\`,
					},
				},
			},
			`escape {`: {
				src: `\{\{`,
				expected: []result{
					{
						pos: 1,
						tok: token.STRING,
						lit: "{{",
					},
				},
			},
			`escape complex`: {
				src: `{\{\\{{`,
				expected: []result{
					{
						pos: 1,
						tok: token.STRING,
						lit: `{{\`,
					},
					{
						pos: 6,
						tok: token.LDBRACE,
						lit: "{{",
					},
				},
			},
			"trailing {": {
				src: "test {",
				expected: []result{
					{
						pos: 1,
						tok: token.STRING,
						lit: "test {",
					},
				},
			},
			"trailing {{": {
				src: "test {{",
				expected: []result{
					{
						pos: 1,
						tok: token.STRING,
						lit: "test ",
					},
					{
						pos: 6,
						tok: token.LDBRACE,
						lit: "{{",
					},
				},
			},
			"trailing {{}}": {
				src: "test {{}}",
				expected: []result{
					{
						pos: 1,
						tok: token.STRING,
						lit: "test ",
					},
					{
						pos: 6,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 8,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"just a STRING": {
				src: `{{ "test" }}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 4,
						tok: token.STRING,
						lit: "test",
					},
					{
						pos: 11,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"just an INT": {
				src: `{{123}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.INT,
						lit: "123",
					},
					{
						pos: 6,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"just a negative INT": {
				src: `{{-123}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.SUB,
						lit: "-",
					},
					{
						pos: 4,
						tok: token.INT,
						lit: "123",
					},
					{
						pos: 7,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"just a FLOAT": {
				src: `{{1.23}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.FLOAT,
						lit: "1.23",
					},
					{
						pos: 7,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"just a FLOAT (first disit is 0)": {
				src: `{{0.123}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.FLOAT,
						lit: "0.123",
					},
					{
						pos: 8,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"just a BOOL": {
				src: `{{true}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 7,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"just a IDENT": {
				src: "{{  test  }}",
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 5,
						tok: token.IDENT,
						lit: "test",
					},
					{
						pos: 11,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"IDENT with special character": {
				src: "{{a-b_c}}",
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.IDENT,
						lit: "a-b_c",
					},
					{
						pos: 8,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"$.a IDENT": {
				src: "{{$.a}}",
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.IDENT,
						lit: "$",
					},
					{
						pos: 4,
						tok: token.PERIOD,
						lit: ".",
					},
					{
						pos: 5,
						tok: token.IDENT,
						lit: "a",
					},
					{
						pos: 6,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"parameter with raw string": {
				src: "prefix-{{test}}-suffix",
				expected: []result{
					{
						pos: 1,
						tok: token.STRING,
						lit: "prefix-",
					},
					{
						pos: 8,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 10,
						tok: token.IDENT,
						lit: "test",
					},
					{
						pos: 14,
						tok: token.RDBRACE,
						lit: "}}",
					},
					{
						pos: 16,
						tok: token.STRING,
						lit: "-suffix",
					},
				},
			},
			"IDENT.IDENT.IDENT": {
				src: "{{test.cases.length}}",
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.IDENT,
						lit: "test",
					},
					{
						pos: 7,
						tok: token.PERIOD,
						lit: ".",
					},
					{
						pos: 8,
						tok: token.IDENT,
						lit: "cases",
					},
					{
						pos: 13,
						tok: token.PERIOD,
						lit: ".",
					},
					{
						pos: 14,
						tok: token.IDENT,
						lit: "length",
					},
					{
						pos: 20,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"IDENT[INT][INT]": {
				src: "{{test[0][100]}}",
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.IDENT,
						lit: "test",
					},
					{
						pos: 7,
						tok: token.LBRACK,
						lit: "[",
					},
					{
						pos: 8,
						tok: token.INT,
						lit: "0",
					},
					{
						pos: 9,
						tok: token.RBRACK,
						lit: "]",
					},
					{
						pos: 10,
						tok: token.LBRACK,
						lit: "[",
					},
					{
						pos: 11,
						tok: token.INT,
						lit: "100",
					},
					{
						pos: 14,
						tok: token.RBRACK,
						lit: "]",
					},
					{
						pos: 15,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"function call": {
				src: "{{ test() }}",
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 4,
						tok: token.IDENT,
						lit: "test",
					},
					{
						pos: 8,
						tok: token.LPAREN,
						lit: "(",
					},
					{
						pos: 9,
						tok: token.RPAREN,
						lit: ")",
					},
					{
						pos: 11,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"function call with args": {
				src: "{{ test(1,2,3) }}",
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 4,
						tok: token.IDENT,
						lit: "test",
					},
					{
						pos: 8,
						tok: token.LPAREN,
						lit: "(",
					},
					{
						pos: 9,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 10,
						tok: token.COMMA,
						lit: ",",
					},
					{
						pos: 11,
						tok: token.INT,
						lit: "2",
					},
					{
						pos: 12,
						tok: token.COMMA,
						lit: ",",
					},
					{
						pos: 13,
						tok: token.INT,
						lit: "3",
					},
					{
						pos: 14,
						tok: token.RPAREN,
						lit: ")",
					},
					{
						pos: 16,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"function call with YAML arg": {
				src: strings.Trim(`
{{test <-}}:
  foo: one
  bar: '{{2}}'
  baz: 3
`, "\n"),
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.IDENT,
						lit: "test",
					},
					{
						pos: 8,
						tok: token.LARROW,
						lit: "<-",
					},
					{
						pos: 10,
						tok: token.RDBRACE,
						lit: "}}",
					},
					{
						pos: 13,
						tok: token.STRING,
						lit: `
  foo: one
  bar: `,
					},
					{
						pos: 33,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 35,
						tok: token.INT,
						lit: "2",
					},
					{
						pos: 36,
						tok: token.RDBRACE,
						lit: "}}",
					},
					{
						pos: 39,
						tok: token.STRING,
						lit: `
  baz: 3`,
					},
					{
						pos: 48,
						tok: token.LINEBREAK,
						lit: "",
					},
				},
			},
			"function call with YAML arg (nest)": {
				src: strings.Trim(`
{{foo <-}}:
  a: 1
  b: |
    {{bar <-}}:
      c: 3
  d: 4
`, "\n"),
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.IDENT,
						lit: "foo",
					},
					{
						pos: 7,
						tok: token.LARROW,
						lit: "<-",
					},
					{
						pos: 9,
						tok: token.RDBRACE,
						lit: "}}",
					},
					{
						pos: 12,
						tok: token.STRING,
						lit: `
  a: 1
  b: |
    `,
					},
					{
						pos: 31,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 33,
						tok: token.IDENT,
						lit: "bar",
					},
					{
						pos: 37,
						tok: token.LARROW,
						lit: "<-",
					},
					{
						pos: 39,
						tok: token.RDBRACE,
						lit: "}}",
					},
					{
						pos: 42,
						tok: token.STRING,
						lit: `
      c: 3`,
					},
					{
						pos: 53,
						tok: token.LINEBREAK,
						lit: "\n  ",
					},
					{
						pos: 56,
						tok: token.STRING,
						lit: `d: 4`,
					},
					{
						pos: 60,
						tok: token.LINEBREAK,
						lit: "",
					},
				},
			},
			"YAML arg function without arg": {
				src: "{{test <-}}",
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.IDENT,
						lit: "test",
					},
					{
						pos: 8,
						tok: token.LARROW,
						lit: "<-",
					},
					{
						pos: 10,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"add": {
				src: `{{"test"+"1"}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.STRING,
						lit: "test",
					},
					{
						pos: 9,
						tok: token.ADD,
						lit: "+",
					},
					{
						pos: 10,
						tok: token.STRING,
						lit: "1",
					},
					{
						pos: 13,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"sub": {
				src: `{{1-2}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 4,
						tok: token.SUB,
						lit: "-",
					},
					{
						pos: 5,
						tok: token.INT,
						lit: "2",
					},
					{
						pos: 6,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"mul": {
				src: `{{1*2}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 4,
						tok: token.MUL,
						lit: "*",
					},
					{
						pos: 5,
						tok: token.INT,
						lit: "2",
					},
					{
						pos: 6,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"quo": {
				src: `{{2/1}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.INT,
						lit: "2",
					},
					{
						pos: 4,
						tok: token.QUO,
						lit: "/",
					},
					{
						pos: 5,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 6,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"rem": {
				src: `{{1%2}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 4,
						tok: token.REM,
						lit: "%",
					},
					{
						pos: 5,
						tok: token.INT,
						lit: "2",
					},
					{
						pos: 6,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"&&": {
				src: `{{true&&true}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 7,
						tok: token.LAND,
						lit: "&&",
					},
					{
						pos: 9,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 13,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"||": {
				src: `{{true||true}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 7,
						tok: token.LOR,
						lit: "||",
					},
					{
						pos: 9,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 13,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"??": {
				src: `{{a.b??"default"}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.IDENT,
						lit: "a",
					},
					{
						pos: 4,
						tok: token.PERIOD,
						lit: ".",
					},
					{
						pos: 5,
						tok: token.IDENT,
						lit: "b",
					},
					{
						pos: 6,
						tok: token.COALESCING,
						lit: "??",
					},
					{
						pos: 8,
						tok: token.STRING,
						lit: "default",
					},
					{
						pos: 17,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"==": {
				src: `{{true==true}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 7,
						tok: token.EQL,
						lit: "==",
					},
					{
						pos: 9,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 13,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"!": {
				src: `{{!true}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.NOT,
						lit: "!",
					},
					{
						pos: 4,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 8,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"!=": {
				src: `{{true!=true}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 7,
						tok: token.NEQ,
						lit: "!=",
					},
					{
						pos: 9,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 13,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"<": {
				src: `{{1<1}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 4,
						tok: token.LSS,
						lit: "<",
					},
					{
						pos: 5,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 6,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"<=": {
				src: `{{1<=1}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 4,
						tok: token.LEQ,
						lit: "<=",
					},
					{
						pos: 6,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 7,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			">": {
				src: `{{1>1}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 4,
						tok: token.GTR,
						lit: ">",
					},
					{
						pos: 5,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 6,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			">=": {
				src: `{{1>=1}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 4,
						tok: token.GEQ,
						lit: ">=",
					},
					{
						pos: 6,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 7,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"conditional expression": {
				src: `{{true ? 1 : 2}}`,
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.BOOL,
						lit: "true",
					},
					{
						pos: 8,
						tok: token.QUESTION,
						lit: "?",
					},
					{
						pos: 10,
						tok: token.INT,
						lit: "1",
					},
					{
						pos: 12,
						tok: token.COLON,
						lit: ":",
					},
					{
						pos: 14,
						tok: token.INT,
						lit: "2",
					},
					{
						pos: 15,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
			"defined": {
				src: "{{defined(a.b)}}",
				expected: []result{
					{
						pos: 1,
						tok: token.LDBRACE,
						lit: "{{",
					},
					{
						pos: 3,
						tok: token.DEFINED,
						lit: "defined",
					},
					{
						pos: 10,
						tok: token.LPAREN,
						lit: "(",
					},
					{
						pos: 11,
						tok: token.IDENT,
						lit: "a",
					},
					{
						pos: 12,
						tok: token.PERIOD,
						lit: ".",
					},
					{
						pos: 13,
						tok: token.IDENT,
						lit: "b",
					},
					{
						pos: 14,
						tok: token.RPAREN,
						lit: ")",
					},
					{
						pos: 15,
						tok: token.RDBRACE,
						lit: "}}",
					},
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				s := newScanner(strings.NewReader(test.src))
				for i, e := range test.expected {
					pos, tok, lit := s.scan()
					if tok == token.EOF {
						t.Errorf("[%d] unexpected EOF", i)
					}
					if got, expected := pos, e.pos; got != expected {
						t.Errorf(`[%d] expected %d but got %d`, i, expected, got)
					}
					if got, expected := tok, e.tok; got != expected {
						t.Errorf(`[%d] expected "%s" but got "%s"`, i, expected, got)
					}
					if got, expected := lit, e.lit; got != expected {
						t.Errorf(`[%d] expected "%s" but got "%s"`, i, expected, got)
					}
					if t.Failed() {
						t.FailNow()
					}
				}
				pos, tok, lit := s.scan()
				if tok != token.EOF {
					t.Fatalf(`expected EOF but got %d:%s:%s`, pos, tok, lit)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			src string
			pos int
			lit string
		}{
			"invalid identifier": {
				src: "{{_a}}",
				pos: 3,
				lit: "_",
			},
			"invalid integer index": {
				src: "{{01}}",
				pos: 3,
				lit: "01",
			},
			"invalid float": {
				src: "{{01.2.3}}",
				pos: 3,
				lit: "01.2.3",
			},
			"invalid float (trailing period)": {
				src: "{{1.}}",
				pos: 3,
				lit: "1.",
			},
			"invalid float (too many period)": {
				src: "{{1.2.3}}",
				pos: 3,
				lit: "1.2.3",
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				s := newScanner(strings.NewReader(test.src))
				for {
					pos, tok, lit := s.scan()
					if tok == token.EOF {
						t.Fatal("unexpected EOF")
					}
					if tok == token.ILLEGAL {
						if got, expected := pos, test.pos; got != expected {
							t.Errorf(`expected %d but got %d`, expected, got)
						}
						if got, expected := lit, test.lit; got != expected {
							t.Errorf(`expected "%s" but got "%s"`, expected, got)
						}
						break
					}
				}
			})
		}
	})
}
