package lsp

import (
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
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
	f.Add("plugins.myPlugin.CreateClient(vars.")
	f.Add("plugins.p.Func(")
	f.Add("plugins.p.Func(vars.x)")

	srv := &Server{}
	f.Fuzz(func(t *testing.T, expr string) {
		srv.completeTemplate(nil, expr)
	})
}

func FuzzTemplateDiagnostics(f *testing.F) {
	f.Add("schemaVersion: scenario/v1\nsteps:\n  - request:\n      url: '{{vars.host'\n")
	f.Add("schemaVersion: scenario/v1\nsteps:\n  - if: '{{steps.x.result == \"passed\" && defined(vars.y) ?? z}}'\n")
	f.Add("schemaVersion: scenario/v1\nsteps:\n  - request:\n      body: |\n        {{plugins.p.F(vars.a[0].b, f()[1])}}\n")
	f.Add("schemaVersion: scenario/v1\nsteps:\n  - request:\n      body: >-\n        a\n        {{assert.x <-}}\n")
	f.Add("schemaVersion: scenario/v1\nsteps:\n  - expect:\n      body:\n        '{{assert.and <-}}':\n        - '{{$ != \"\"}}'\n")
	f.Add("schemaVersion: scenario/v1\nvars:\n  a: \"日本\\\"{{hots}}\\n{{\"\n")
	f.Add("schemaVersion: config/v1\nvars:\n  a: '{{vars.'\n---\nschemaVersion: scenario/v1\nanchors:\n  x: &x '{{1 + }}'\n")
	f.Add("schemaVersion: scenario/v1\nsteps:\n- vars: {a: '{{a}}', b: [\"{{b}\"]}\n")
	f.Add("schemaVersion: scenario/v1\nanchors:\n  s: &s {id: a}\nsteps:\n- *s\n- <<: [*s, *t]\n  if: '{{steps.a.result ?? (b) ?? f()[1] ?? defined()}}'\n- include: x.yaml\n  bind: {vars: {a: '{{steps.a == !}}'}}\n")
	f.Add("schemaVersion: ytt/v1\nfiles: [a]\n---\nschemaVersion: config/v1\nprotocols: {grpc: {request: '{{1 + }}'}}\n")

	f.Fuzz(func(t *testing.T, text string) {
		doc := &document{Text: text, Parsed: yamlutil.Parse(text)}
		lines := strings.Split(text, "\n")
		for _, d := range (&Server{}).templateDiagnostics(doc) {
			for _, p := range []Position{d.Range.Start, d.Range.End} {
				if p.Line < 0 || p.Line >= len(lines) || p.Character < 0 || p.Character > len(lines[p.Line]) {
					t.Fatalf("diagnostic %q at %+v is outside the document", d.Message, d.Range)
				}
			}
		}
	})
}
