package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/template/ast"
	"github.com/scenarigo/scenarigo/template/token"
)

func TestParser_Parse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			src      string
			expected ast.Expr
		}{
			"only string": {
				src: "only string",
				expected: &ast.BasicLit{
					ValuePos: 1,
					Kind:     token.STRING,
					Value:    "only string",
				},
			},
			"empty parameter": {
				src: "{{}}",
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					Rdbrace: 3,
				},
			},
			"just a string": {
				src: `{{"test"}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BasicLit{
						ValuePos: 3,
						Kind:     token.STRING,
						Value:    "test",
					},
					Rdbrace: 9,
				},
			},
			"just an integer": {
				src: `{{123}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BasicLit{
						ValuePos: 3,
						Kind:     token.INT,
						Value:    "123",
					},
					Rdbrace: 6,
				},
			},
			"just a negative integer": {
				src: `{{-123}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.UnaryExpr{
						OpPos: 3,
						Op:    token.SUB,
						X: &ast.BasicLit{
							ValuePos: 4,
							Kind:     token.INT,
							Value:    "123",
						},
					},
					Rdbrace: 7,
				},
			},
			"just a float": {
				src: `{{1.23}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BasicLit{
						ValuePos: 3,
						Kind:     token.FLOAT,
						Value:    "1.23",
					},
					Rdbrace: 7,
				},
			},
			"just a bool": {
				src: `{{true}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BasicLit{
						ValuePos: 3,
						Kind:     token.BOOL,
						Value:    "true",
					},
					Rdbrace: 7,
				},
			},
			"nil keyword": {
				src: `{{nil}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BasicLit{
						ValuePos: 3,
						Kind:     token.NIL,
						Value:    "nil",
					},
					Rdbrace: 6,
				},
			},
			"null keyword": {
				src: `{{null}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BasicLit{
						ValuePos: 3,
						Kind:     token.NIL,
						Value:    "null",
					},
					Rdbrace: 7,
				},
			},
			"not bool": {
				src: `{{!true}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.UnaryExpr{
						OpPos: 3,
						Op:    token.NOT,
						X: &ast.BasicLit{
							ValuePos: 4,
							Kind:     token.BOOL,
							Value:    "true",
						},
					},
					Rdbrace: 8,
				},
			},
			"parenthesized expression": {
				src: "{{(1)}}",
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.ParenExpr{
						Lparen: 3,
						X: &ast.BasicLit{
							ValuePos: 4,
							Kind:     token.INT,
							Value:    "1",
						},
						Rparen: 5,
					},
					Rdbrace: 6,
				},
			},
			"just a parameter": {
				src: "{{test}}",
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.Ident{
						NamePos: 3,
						Name:    "test",
					},
					Rdbrace: 7,
				},
			},
			"$ ident": {
				src: "{{$}}",
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.Ident{
						NamePos: 3,
						Name:    "$",
					},
					Rdbrace: 4,
				},
			},
			"multi parameter": {
				src: "{{one}}{{two}}{{three}}",
				expected: &ast.BinaryExpr{
					X: &ast.BinaryExpr{
						X: &ast.ParameterExpr{
							Ldbrace: 1,
							X: &ast.Ident{
								NamePos: 3,
								Name:    "one",
							},
							Rdbrace: 6,
						},
						OpPos: 8,
						Op:    token.CONCAT,
						Y: &ast.ParameterExpr{
							Ldbrace: 8,
							X: &ast.Ident{
								NamePos: 10,
								Name:    "two",
							},
							Rdbrace: 13,
						},
					},
					OpPos: 15,
					Op:    token.CONCAT,
					Y: &ast.ParameterExpr{
						Ldbrace: 15,
						X: &ast.Ident{
							NamePos: 17,
							Name:    "three",
						},
						Rdbrace: 22,
					},
				},
			},
			"parameter with raw string": {
				src: "prefix-{{test}}-suffix",
				expected: &ast.BinaryExpr{
					X: &ast.BinaryExpr{
						X: &ast.BasicLit{
							ValuePos: 1,
							Kind:     token.STRING,
							Value:    "prefix-",
						},
						OpPos: 8,
						Op:    token.CONCAT,
						Y: &ast.ParameterExpr{
							Ldbrace: 8,
							X: &ast.Ident{
								NamePos: 10,
								Name:    "test",
							},
							Rdbrace: 14,
						},
					},
					OpPos: 16,
					Op:    token.CONCAT,
					Y: &ast.BasicLit{
						ValuePos: 16,
						Kind:     token.STRING,
						Value:    "-suffix",
					},
				},
			},
			"selector": {
				src: "{{test.cases.length}}",
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.SelectorExpr{
						X: &ast.SelectorExpr{
							X: &ast.Ident{
								NamePos: 3,
								Name:    "test",
							},
							Sel: &ast.Ident{
								NamePos: 8,
								Name:    "cases",
							},
						},
						Sel: &ast.Ident{
							NamePos: 14,
							Name:    "length",
						},
					},
					Rdbrace: 20,
				},
			},
			"index": {
				src: "{{test[0][100]}}",
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.IndexExpr{
						X: &ast.IndexExpr{
							X: &ast.Ident{
								NamePos: 3,
								Name:    "test",
							},
							Lbrack: 7,
							Index: &ast.BasicLit{
								ValuePos: 8,
								Kind:     token.INT,
								Value:    "0",
							},
							Rbrack: 9,
						},
						Lbrack: 10,
						Index: &ast.BasicLit{
							ValuePos: 11,
							Kind:     token.INT,
							Value:    "100",
						},
						Rbrack: 14,
					},
					Rdbrace: 15,
				},
			},
			"function call": {
				src: "{{test(1,2)}}",
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.CallExpr{
						Fun: &ast.Ident{
							NamePos: 3,
							Name:    "test",
						},
						Lparen: 7,
						Args: []ast.Expr{
							&ast.BasicLit{
								ValuePos: 8,
								Kind:     token.INT,
								Value:    "1",
							},
							&ast.BasicLit{
								ValuePos: 10,
								Kind:     token.INT,
								Value:    "2",
							},
						},
						Rparen: 11,
					},
					Rdbrace: 12,
				},
			},
			"function call with YAML arg": {
				src: strings.Trim(`
{{echo <-}}:
  message: '{{message}}'
`, "\n"),
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.LeftArrowExpr{
						Fun: &ast.Ident{
							NamePos: 3,
							Name:    "echo",
						},
						Larrow:  8,
						Rdbrace: 10,
						Arg: &ast.BinaryExpr{
							X: &ast.BasicLit{
								ValuePos: 13,
								Kind:     token.STRING,
								Value:    "\n  message: ",
							},
							OpPos: 26,
							Op:    token.CONCAT,
							Y: &ast.ParameterExpr{
								Ldbrace: 26,
								X: &ast.Ident{
									NamePos: 28,
									Name:    "message",
								},
								Rdbrace: 35,
								Quoted:  true,
							},
						},
					},
					Rdbrace: 10,
				},
			},
			"function call with YAML arg (nest)": {
				src: strings.Trim(`
{{join <-}}:
  prefix: preout-
  text: |-
    {{join <-}}:
      prefix: prein-
      text: '{{text}}'
      suffix: -sufin
  suffix: -sufout
`, "\n"),
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.LeftArrowExpr{
						Fun: &ast.Ident{
							NamePos: 3,
							Name:    "join",
						},
						Larrow:  8,
						Rdbrace: 10,
						Arg: &ast.BinaryExpr{
							X: &ast.BinaryExpr{
								X: &ast.BinaryExpr{
									X: &ast.BasicLit{
										ValuePos: 13,
										Kind:     token.STRING,
										Value: `
  prefix: preout-
  text: |-
    `,
									},
									OpPos: 47,
									Op:    token.CONCAT,
									Y: &ast.ParameterExpr{
										Ldbrace: 47,
										X: &ast.LeftArrowExpr{
											Fun: &ast.Ident{
												NamePos: 49,
												Name:    "join",
											},
											Larrow:  54,
											Rdbrace: 56,
											Arg: &ast.BinaryExpr{
												X: &ast.BinaryExpr{
													X: &ast.BasicLit{
														ValuePos: 59,
														Kind:     token.STRING,
														Value: `
      prefix: prein-
      text: `,
													},
													OpPos: 94,
													Op:    token.CONCAT,
													Y: &ast.ParameterExpr{
														Ldbrace: 94,
														X: &ast.Ident{
															NamePos: 96,
															Name:    "text",
														},
														Rdbrace: 100,
														Quoted:  true,
													},
												},
												OpPos: 103,
												Op:    token.CONCAT,
												Y: &ast.BasicLit{
													ValuePos: 103,
													Kind:     token.STRING,
													Value: `
      suffix: -sufin`,
												},
											},
										},
										Rdbrace: 56,
									},
								},
								OpPos: 124,
								Op:    token.CONCAT,
								Y: &ast.BasicLit{
									ValuePos: 124,
									Kind:     token.STRING,
									Value:    "\n  ",
								},
							},
							OpPos: 127,
							Op:    token.CONCAT,
							Y: &ast.BasicLit{
								ValuePos: 127,
								Kind:     token.STRING,
								Value:    "suffix: -sufout",
							},
						},
					},
					Rdbrace: 10,
				},
			},
			"YAML arg function without arg": {
				src: "{{test <-}}",
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.LeftArrowExpr{
						Fun: &ast.Ident{
							NamePos: 3,
							Name:    "test",
						},
						Larrow:  8,
						Rdbrace: 10,
						Arg:     nil,
					},
					Rdbrace: 10,
				},
			},
			"add": {
				src: `{{"foo"+"-"+"1"}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.BinaryExpr{
							X: &ast.BasicLit{
								ValuePos: 3,
								Kind:     token.STRING,
								Value:    "foo",
							},
							OpPos: 8,
							Op:    token.ADD,
							Y: &ast.BasicLit{
								ValuePos: 9,
								Kind:     token.STRING,
								Value:    "-",
							},
						},
						OpPos: 12,
						Op:    token.ADD,
						Y: &ast.BasicLit{
							ValuePos: 13,
							Kind:     token.STRING,
							Value:    "1",
						},
					},
					Rdbrace: 16,
				},
			},
			"sub": {
				src: `{{1-2}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.INT,
							Value:    "1",
						},
						OpPos: 4,
						Op:    token.SUB,
						Y: &ast.BasicLit{
							ValuePos: 5,
							Kind:     token.INT,
							Value:    "2",
						},
					},
					Rdbrace: 6,
				},
			},
			"&&": {
				src: `{{true&&true}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.BOOL,
							Value:    "true",
						},
						OpPos: 7,
						Op:    token.LAND,
						Y: &ast.BasicLit{
							ValuePos: 9,
							Kind:     token.BOOL,
							Value:    "true",
						},
					},
					Rdbrace: 13,
				},
			},
			"||": {
				src: `{{true||true}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.BOOL,
							Value:    "true",
						},
						OpPos: 7,
						Op:    token.LOR,
						Y: &ast.BasicLit{
							ValuePos: 9,
							Kind:     token.BOOL,
							Value:    "true",
						},
					},
					Rdbrace: 13,
				},
			},
			"??": {
				src: `{{a.b??"default"}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.SelectorExpr{
							X: &ast.Ident{
								NamePos: 3,
								Name:    "a",
							},
							Sel: &ast.Ident{
								NamePos: 5,
								Name:    "b",
							},
						},
						OpPos: 6,
						Op:    token.COALESCING,
						Y: &ast.BasicLit{
							ValuePos: 8,
							Kind:     token.STRING,
							Value:    "default",
						},
					},
					Rdbrace: 17,
				},
			},
			"==": {
				src: `{{1==1}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.INT,
							Value:    "1",
						},
						OpPos: 4,
						Op:    token.EQL,
						Y: &ast.BasicLit{
							ValuePos: 6,
							Kind:     token.INT,
							Value:    "1",
						},
					},
					Rdbrace: 7,
				},
			},
			"!=": {
				src: `{{1!=1}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.INT,
							Value:    "1",
						},
						OpPos: 4,
						Op:    token.NEQ,
						Y: &ast.BasicLit{
							ValuePos: 6,
							Kind:     token.INT,
							Value:    "1",
						},
					},
					Rdbrace: 7,
				},
			},
			"<": {
				src: `{{1<1}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.INT,
							Value:    "1",
						},
						OpPos: 4,
						Op:    token.LSS,
						Y: &ast.BasicLit{
							ValuePos: 5,
							Kind:     token.INT,
							Value:    "1",
						},
					},
					Rdbrace: 6,
				},
			},
			"<=": {
				src: `{{1<=1}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.INT,
							Value:    "1",
						},
						OpPos: 4,
						Op:    token.LEQ,
						Y: &ast.BasicLit{
							ValuePos: 6,
							Kind:     token.INT,
							Value:    "1",
						},
					},
					Rdbrace: 7,
				},
			},
			">": {
				src: `{{1>1}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.INT,
							Value:    "1",
						},
						OpPos: 4,
						Op:    token.GTR,
						Y: &ast.BasicLit{
							ValuePos: 5,
							Kind:     token.INT,
							Value:    "1",
						},
					},
					Rdbrace: 6,
				},
			},
			">=": {
				src: `{{1>=1}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.BinaryExpr{
						X: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.INT,
							Value:    "1",
						},
						OpPos: 4,
						Op:    token.GEQ,
						Y: &ast.BasicLit{
							ValuePos: 6,
							Kind:     token.INT,
							Value:    "1",
						},
					},
					Rdbrace: 7,
				},
			},
			"conditional expression": {
				src: `{{true?1:2}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.ConditionalExpr{
						Condition: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.BOOL,
							Value:    "true",
						},
						Question: 7,
						X: &ast.BasicLit{
							ValuePos: 8,
							Kind:     token.INT,
							Value:    "1",
						},
						Colon: 9,
						Y: &ast.BasicLit{
							ValuePos: 10,
							Kind:     token.INT,
							Value:    "2",
						},
					},
					Rdbrace: 11,
				},
			},
			"defined()": {
				src: `{{defined(a.b)}}`,
				expected: &ast.ParameterExpr{
					Ldbrace: 1,
					X: &ast.DefinedExpr{
						DefinedPos: 3,
						Lparen:     10,
						Arg: &ast.SelectorExpr{
							X: &ast.Ident{
								NamePos: 11,
								Name:    "a",
							},
							Sel: &ast.Ident{
								NamePos: 13,
								Name:    "b",
							},
						},
						Rparen: 14,
					},
					Rdbrace: 15,
				},
			},
			"expr with new-line-char": {
				src: `
{{foo(
  1,
  3
  -
  2)}}
`,
				expected: &ast.BinaryExpr{
					X: &ast.BinaryExpr{
						X:     &ast.BasicLit{ValuePos: 1, Kind: token.STRING, Value: "\n"},
						OpPos: 2,
						Op:    token.CONCAT,
						Y: &ast.ParameterExpr{
							Ldbrace: 2,
							X: &ast.CallExpr{
								Fun:    &ast.Ident{NamePos: 4, Name: "foo"},
								Lparen: 7,
								Args: []ast.Expr{
									&ast.BasicLit{ValuePos: 11, Kind: token.INT, Value: "1"},
									&ast.BinaryExpr{
										X:     &ast.BasicLit{ValuePos: 16, Kind: token.INT, Value: "3"},
										OpPos: 20,
										Op:    token.SUB,
										Y:     &ast.BasicLit{ValuePos: 24, Kind: token.INT, Value: "2"},
									},
								},
								Rparen: 25,
							},
							Rdbrace: 26,
						},
					},
					OpPos: 28,
					Op:    token.CONCAT,
					Y:     &ast.BasicLit{ValuePos: 28, Kind: token.STRING, Value: "\n"},
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				p := NewParser(strings.NewReader(test.src))
				got, err := p.Parse()
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.expected, got); diff != "" {
					t.Errorf("result differs: (-want +got)\n%s", diff)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			src string
			pos int
		}{
			"}} not found": {
				src: "{{ test",
				pos: 8,
			},
			"] not found": {
				src: "{{ test[2 }}",
				pos: 11,
			},
			"no parent": {
				src: "{{ .key }}",
				pos: 4,
			},
			"repeated .": {
				src: "{{ test..key }}",
				pos: 9,
			},
			"selector index after .": {
				src: "{{ test.[0] }}",
				pos: 9,
			},
			"invalid $$ ident": {
				src: "{{$$}}",
				pos: 4,
			},
			"invalid $a ident": {
				src: "{{$a}}",
				pos: 4,
			},
			"invalid a$ ident": {
				src: "{{a$}}",
				pos: 4,
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				p := NewParser(strings.NewReader(test.src))
				_, err := p.Parse()
				if err == nil {
					t.Fatal("expected error")
				}
				var errs Errors
				if ok := errors.As(err, &errs); !ok {
					t.Fatalf("expected parse errors: %s", err)
				}
				if got, expected := errs[0].pos, test.pos; got != expected {
					t.Fatalf("expected %d but got %d: %s", expected, got, err)
				}
			})
		}
	})
}
