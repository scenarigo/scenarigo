package lsp

import (
	"testing"
)

func FuzzGetTemplateContext(f *testing.F) {
	f.Add("schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - request:\n      url: \"{{vars.\"\n", 4, 18)
	f.Add("url: \"{{\"\n", 0, 8)
	f.Add("", 0, 0)
	f.Add("no template here", 0, 5)
	f.Add("line1\nline2\n{{partial", 2, 9)

	f.Fuzz(func(t *testing.T, text string, line, char int) {
		if line < 0 || char < 0 {
			return
		}
		getTemplateContext(text, Position{Line: line, Character: char})
	})
}

func FuzzCompleteTemplate(f *testing.F) {
	f.Add("")
	f.Add("vars")
	f.Add("assert.")
	f.Add("assert.contains")
	f.Add("unknown.field.deep")
	f.Add("...")
	f.Add("a.b.c.d.e")

	srv := &Server{}
	f.Fuzz(func(t *testing.T, expr string) {
		srv.completeTemplate(expr)
	})
}
