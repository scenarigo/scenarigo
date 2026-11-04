package lsp

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestNameChains(t *testing.T) {
	tests := []struct {
		expr string
		want []string // "path@start-end"
	}{
		{expr: "vars.user", want: []string{"vars.user@0-9"}},
		{expr: " vars.user.name ", want: []string{"vars.user.name@1-15"}},
		{expr: `vars.user ?? "x"`, want: []string{"vars.user@0-9"}},
		{expr: "size(vars.user) > 0", want: []string{"vars.user@5-14"}},
		{expr: "plugins.p.F(vars.a, secrets.b)", want: []string{"plugins.p.F@0-11", "vars.a@12-18", "secrets.b@20-29"}},
		{expr: "assert.contains <-", want: []string{"assert.contains@0-15"}},
		{expr: `"vars.user" + vars.x`, want: []string{"vars.x@14-20"}},
		{expr: `"unterminated vars.user`, want: nil},
		{expr: "vars", want: nil},
		{expr: "1.5 + vars.my-var", want: []string{"vars.my-var@6-17"}},
		{expr: "$.id", want: []string{"$.id@0-4"}},
		{expr: "vars.", want: nil},
		{expr: "vars[0].x", want: nil},
	}
	for _, tt := range tests {
		var got []string
		for _, c := range nameChains(tt.expr) {
			got = append(got, fmt.Sprintf("%s@%d-%d", joinPath(c.path), c.start, c.end))
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("nameChains(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func joinPath(path []string) string {
	return strings.Join(path, ".")
}

// TestValueMapFallback checks the range of a value that cannot be aligned
// with its source: the rest of the first line of the scalar.
func TestValueMapFallback(t *testing.T) {
	text := "key: abc\nnext: 1\n"
	lines := newLineIndex(text)
	m := newValueMap(text, lines, 5, "xyz")
	if len(m.offsets) != 0 {
		t.Fatalf("expected no alignment, got %v", m.offsets)
	}
	want := Range{Start: Position{Line: 0, Character: 5}, End: Position{Line: 0, Character: 8}}
	if got := m.runeRange(1, 2); got != want {
		t.Errorf("range = %+v, want %+v", got, want)
	}
}

func TestDecodeYAMLEscape(t *testing.T) {
	tests := []struct {
		in   string
		want rune
		n    int
	}{
		{`\n`, '\n', 2}, {`\"`, '"', 2}, {`\\`, '\\', 2}, {`\x41`, 'A', 4}, {`\u00e9`, 'é', 6}, {`\U0001F600`, '😀', 10},
	}
	for _, tt := range tests {
		got, n, ok := decodeYAMLEscape(tt.in + "rest")
		if !ok || got != tt.want || n != tt.n {
			t.Errorf("decodeYAMLEscape(%q) = %q, %d, %v; want %q, %d", tt.in, got, n, ok, tt.want, tt.n)
		}
	}
	for _, bad := range []string{`\`, `\q`, `\x4`, `\uZZZZ`, "x"} {
		if _, _, ok := decodeYAMLEscape(bad); ok {
			t.Errorf("decodeYAMLEscape(%q) accepted", bad)
		}
	}
}
