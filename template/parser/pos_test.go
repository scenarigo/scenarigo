package parser

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParser_Pos(t *testing.T) {
	tests := map[string]struct {
		in     string
		pos    int
		expect *Position
	}{
		"first rune": {
			in:  "てすと",
			pos: 1,
			expect: &Position{
				Line:   1,
				Column: 1,
				Offset: 1,
			},
		},
		"second rune": {
			in:  "てすと",
			pos: 2,
			expect: &Position{
				Line:   1,
				Column: 2,
				Offset: 2,
			},
		},
		"second line": {
			in: strings.Trim(`
test
てすと
テスト
`, "\n"),
			pos: 7,
			expect: &Position{
				Line:   2,
				Column: 2,
				Offset: 7,
			},
		},
		"last line": {
			in: strings.Trim(`
test
てすと
テスト
`, "\n"),
			pos: 12,
			expect: &Position{
				Line:   3,
				Column: 3,
				Offset: 12,
			},
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			p := NewParser(strings.NewReader(test.in))
			if _, err := p.Parse(); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(test.expect, p.Pos(test.pos)); diff != "" {
				t.Errorf("result differs: (-want +got)\n%s", diff)
			}
		})
	}
}
