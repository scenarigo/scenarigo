package parser

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zoncoen/scenarigo/template/ast"
	"github.com/zoncoen/scenarigo/template/token"
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
						Op:    token.ADD,
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
					Op:    token.ADD,
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
						Op:    token.ADD,
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
					Op:    token.ADD,
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
							Op:    token.ADD,
							Y: &ast.ParameterExpr{
								Ldbrace: 26,
								X: &ast.Ident{
									NamePos: 28,
									Name:    "message",
								},
								Rdbrace: 35,
							},
						},
					},
					Rdbrace: 10,
				},
			},
			"function call with YAML arg (nest)": {
				src: strings.Trim(`
{{echo <-}}:
  message: |
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
								Value:    "\n  message: |\n    ",
							},
							OpPos: 31,
							Op:    token.ADD,
							Y: &ast.ParameterExpr{
								Ldbrace: 31,
								X: &ast.LeftArrowExpr{
									Fun: &ast.Ident{
										NamePos: 33,
										Name:    "echo",
									},
									Larrow:  38,
									Rdbrace: 40,
									Arg: &ast.BinaryExpr{
										X: &ast.BasicLit{
											ValuePos: 43,
											Kind:     token.STRING,
											Value:    "\n      message: ",
										},
										OpPos: 60,
										Op:    token.ADD,
										Y: &ast.ParameterExpr{
											Ldbrace: 60,
											X: &ast.Ident{
												NamePos: 62,
												Name:    "message",
											},
											Rdbrace: 69,
										},
									},
								},
								Rdbrace: 40,
							},
						},
					},
					Rdbrace: 10,
				},
			},
			"function call with YAML arg (complex)": {
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
									Op:    token.ADD,
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
													Op:    token.ADD,
													Y: &ast.ParameterExpr{
														Ldbrace: 94,
														X: &ast.Ident{
															NamePos: 96,
															Name:    "text",
														},
														Rdbrace: 100,
													},
												},
												OpPos: 103,
												Op:    token.ADD,
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
								Op:    token.ADD,
								Y: &ast.BasicLit{
									ValuePos: 124,
									Kind:     token.STRING,
									Value:    "\n  ",
								},
							},
							OpPos: 127,
							Op:    token.ADD,
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
		}
		for name, test := range tests {
			test := test
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
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				p := NewParser(strings.NewReader(test.src))
				_, err := p.Parse()
				if err == nil {
					t.Fatal("expected error")
				}
				errs, ok := err.(Errors)
				if !ok {
					t.Fatalf("expected parse errors: %s", err)
				}
				if got, expected := errs[0].pos, test.pos; got != expected {
					t.Fatalf("expected %d but got %d: %s", expected, got, err)
				}
			})
		}
	})
}
