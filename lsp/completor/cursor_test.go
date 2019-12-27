package completor

import (
	"testing"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestFinder_find(t *testing.T) {
	tests := map[string]struct {
		file   string
		finder *finder
		expect *cursor
	}{
		"minimum": {
			file: "testdata/minimum.yaml",
			finder: &finder{
				line:   1,
				column: 1,
			},
			expect: &cursor{
				node: &ast.StringNode{
					Token: &token.Token{
						Type:          token.StringType,
						CharacterType: token.CharacterTypeMiscellaneous,
						Value:         "a",
						Origin:        "a",
						Position: &token.Position{
							Line:   1,
							Column: 1,
							Offset: 2,
						},
					},
					Value: "a",
				},
				parent: &cursor{
					node: &ast.Document{},
				},
			},
		},
		"map key": {
			file: "testdata/map.yaml",
			finder: &finder{
				line:   1,
				column: 1,
			},
			expect: &cursor{
				node: &ast.StringNode{
					Token: &token.Token{
						Type:          token.StringType,
						CharacterType: token.CharacterTypeMiscellaneous,
						Value:         "k",
						Origin:        "k",
						Position: &token.Position{
							Line:   1,
							Column: 1,
							Offset: 1,
						},
					},
					Value: "k",
				},
				parent: &cursor{
					node: &ast.MappingValueNode{
						Start: &token.Token{
							Type:      token.MappingValueType,
							Indicator: token.BlockStructureIndicator,
							Value:     ":",
							Origin:    ":",
							Position: &token.Position{
								Line:   1,
								Column: 2,
								Offset: 2,
							},
						},
					},
					parent: &cursor{
						node: &ast.Document{},
					},
				},
			},
		},
		"map value": {
			file: "testdata/map.yaml",
			finder: &finder{
				line:   1,
				column: 4,
			},
			expect: &cursor{
				node: &ast.StringNode{
					Token: &token.Token{
						Type:          token.StringType,
						CharacterType: token.CharacterTypeMiscellaneous,
						Value:         "v",
						Origin:        " v",
						Position: &token.Position{
							Line:   1,
							Column: 4,
							Offset: 5,
						},
					},
					Value: "v",
				},
				parent: &cursor{
					node: &ast.MappingValueNode{
						Start: &token.Token{
							Type:      token.MappingValueType,
							Indicator: token.BlockStructureIndicator,
							Value:     ":",
							Origin:    ":",
							Position: &token.Position{
								Line:   1,
								Column: 2,
								Offset: 2,
							},
						},
					},
					parent: &cursor{
						node: &ast.Document{},
					},
				},
			},
		},
		"map value (null value)": {
			file: "testdata/map-without-value.yaml",
			finder: &finder{
				line:   1,
				column: 4,
			},
			expect: &cursor{
				node: &ast.NullNode{
					Token: &token.Token{
						Type:          token.NullType,
						CharacterType: token.CharacterTypeMiscellaneous,
						Value:         "null",
						Origin:        "null",
						Position: &token.Position{
							Line:   1,
							Column: 2,
							Offset: 2,
						},
					},
				},
				parent: &cursor{
					node: &ast.MappingValueNode{
						Start: &token.Token{
							Type:      token.MappingValueType,
							Indicator: token.BlockStructureIndicator,
							Value:     ":",
							Origin:    ":",
							Position: &token.Position{
								Line:   1,
								Column: 2,
								Offset: 2,
							},
						},
					},
					parent: &cursor{
						node: &ast.Document{},
					},
				},
			},
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(test.file, parser.ParseComments)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if l := len(file.Docs); l != 1 {
				t.Fatalf("expected number of document is 1 but got %d", l)
			}
			for _, doc := range file.Docs {
				got := test.finder.find(doc)
				if diff := cmp.Diff(test.expect, got,
					cmp.AllowUnexported(cursor{}),
					cmpopts.IgnoreFields(token.Token{}, "Next", "Prev"),
					cmpopts.IgnoreFields(ast.Document{}, "Body"),
					cmpopts.IgnoreFields(ast.MappingValueNode{}, "Key", "Value"),
				); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
			}
		})
	}
}
