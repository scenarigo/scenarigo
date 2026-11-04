package lsp

import (
	"fmt"
	"slices"
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
	out := path[0]
	for _, p := range path[1:] {
		out += "." + p
	}
	return out
}
