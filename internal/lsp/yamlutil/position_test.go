package yamlutil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/goccy/go-yaml/lexer"
)

func TestGetCursorContext_StepKeys(t *testing.T) {
	text := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    "
	doc := Parse(text)
	if doc == nil {
		t.Fatal("parse failed")
	}

	// Line 5 (0-based), char 4 — inside a step, should be key context
	ctx := doc.GetCursorContext(5, 4)
	t.Logf("Type: %d (0=unknown, 1=key, 2=value)\n", ctx.Type)
	t.Logf("Path: %v\n", ctx.Path)
	t.Logf("ParentKeys: %v\n", ctx.ParentKeys)
	t.Logf("PartialKey: %q\n", ctx.PartialKey)

	if ctx.Type != CursorContextKey {
		t.Errorf("expected key context, got %d", ctx.Type)
	}
}

func TestGetCursorContext_ProtocolValue(t *testing.T) {
	text := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: "
	doc := Parse(text)
	if doc == nil {
		t.Fatal("parse failed")
	}

	ctx := doc.GetCursorContext(4, 15)
	t.Logf("Type: %d\n", ctx.Type)
	t.Logf("Path: %v\n", ctx.Path)
	t.Logf("PartialValue: %q\n", ctx.PartialValue)

	if ctx.Type != CursorContextValue {
		t.Errorf("expected value context, got %d", ctx.Type)
	}
}

func TestFindNodeAtPosition_FlowMapping(t *testing.T) {
	doc := Parse("steps:\n  - {title: 日本語, protocol: http}\n")
	if doc == nil {
		t.Fatal("parse failed")
	}
	// Columns are 1-based rune indexes: "  - {title: 日本語, protocol: http}"
	tests := []struct {
		col  int
		want string
	}{
		{col: 6, want: "title"},     // on "title"
		{col: 13, want: "title"},    // on the value of title
		{col: 19, want: "protocol"}, // on "protocol"
		{col: 29, want: "protocol"}, // on the value of protocol
		{col: 40, want: "title"},    // past the end of the line: first key on the line, as before
	}
	for _, tt := range tests {
		path := doc.FindNodeAtPosition(2, tt.col)
		if path == nil {
			t.Errorf("col %d: no node", tt.col)
			continue
		}
		if got := path.Keys[len(path.Keys)-1]; got != tt.want {
			t.Errorf("col %d: key = %q (path %v), want %q", tt.col, got, path.Keys, tt.want)
		}
	}
}

func TestParse_ParserPanic(t *testing.T) {
	tests := map[string]string{
		"a value":  "%TAG !! tag:example.com,2000:\n---\na: !!str\n",
		"an entry": "%TAG !! tag:example.com,2000:\n---\n- !!str\n",
		"a root":   "%TAG !! tag:x:\n--- ! ",
	}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			if doc := Parse(text); doc != nil {
				t.Errorf("Parse returned a document for %q", text)
			}
		})
	}
}

// The texts refused here stay small enough that a parser reached by mistake
// takes a few hundred MiB, not the GiB the bounds exist to prevent.
func TestParse_Cost(t *testing.T) {
	key := strings.Repeat("k", 1<<20)
	items := strings.Repeat
	nestedKeys := func() string {
		var b strings.Builder
		k := strings.Repeat("k", 4096)
		for i := range 64 {
			b.WriteString(strings.Repeat(" ", i) + k + ":\n")
		}
		return b.String() + items(strings.Repeat(" ", 64)+"- 1\n", 2000)
	}
	numbered := func(format string) string {
		var b strings.Builder
		for i := range 5000 {
			fmt.Fprintf(&b, format, i)
		}
		return b.String()
	}
	refused := map[string]string{
		"flow sequences":                  "a: " + items("[", 20000) + items("]", 20000),
		"flow mappings":                   "a: " + items("{a: ", 20000) + "1" + items("}", 20000),
		"sequences on a line":             items("- ", 20000) + "a",
		"both kinds":                      items("- [", 10000) + items("]", 10000),
		"indented sequence":               key + ":\n" + items("  - 1\n", 300),
		"unindented sequence":             key + ":\n" + items("- 1\n", 300),
		"flow under a key":                key + ": [" + items("1, ", 300) + "1]",
		"mapping under a key":             key + ":\n" + strings.ReplaceAll(keys(300), "a", "  a"),
		"tagged key":                      "!!str " + key + ":\n" + items("  - 1\n", 300),
		"tagged key, unindented sequence": "!!str " + key + ":\n" + items("- 1\n", 300),
		"anchored key":                    "&a " + key + ":\n" + items("  - 1\n", 300),
		"explicit key":                    "? " + key + "\n:\n" + items(" - 1\n", 300),
		"tagged key above a key":          "!!str " + key + ":\n  b:\n" + items("    - 1\n", 300),
		"explicit key of an entry":        "- ? " + key + "\n  :\n" + items("    - 1\n", 300),
		"tagged key of an entry":          "- !!str " + key + ":\n" + items("    - 1\n", 300),
		"comment after a key":             key + " # c\n:\n" + items("  - 1\n", 300),
		"comment after an explicit key":   "? " + key + " # c\n:\n" + items("  - 1\n", 300),
		"comment in a flow":               "{? " + key + " # c\n: [" + items("1, ", 300) + "1]}",
		"many keys in a path":             nestedKeys(),
		"document markers":                items("...\n", maxDocuments+1),
		"markers after an alias":          items("*&\n---\n", maxDocuments+1),
		"tags before entries":             items("!a - ", 20000) + "a",
		"anchored tags after keys":        items("k: &a !a\n", 20000) + "a",
		"a tag before entries":            "!a " + items("- ", 20000) + "a",
		"tags after entries":              items("- !a\n", 20000) + "a",
		"tags after keys":                 items("k: !a\n", 20000) + "a",
		"tags before keys":                items("!a\nk: ", 20000) + "a",
		"tags in entries of a key":        items("k:\n- !a\n", 20000) + "a",
		"tags on a line":                  "a: " + items("!!str ", maxChain+1) + "1",
		"tags on lines":                   "a:\n" + items("  !!str\n", maxChain+1) + "  1",
		"tags and comments":               "a:\n" + items("  !!str # c\n", maxChain+1) + "  1",
		"tags and anchors":                "a: " + items("!a &a ", maxChain) + "1",
		"anchors and tags":                "a: " + items("&a !a ", maxChain) + "1",
	}
	for name, text := range refused {
		t.Run(name, func(t *testing.T) {
			// The parser rejects some of these too, but only after it has run.
			if !tooCostly(lexer.Tokenize(text)) {
				t.Error("not refused before the parser")
			}
			if Parse(text) != nil {
				t.Error("parsed")
			}
		})
	}

	n := maxChain + 1
	accepted := map[string]string{
		"markers at the limit":         items("...\n", maxDocuments),
		"tagged keys":                  numbered("!!str k%d: 1\n"),
		"tagged values":                numbered("k%d: !!str a\n"),
		"tagged entries":               numbered("- !!str %d\n"),
		"tagged mappings":              numbered("- !!map\n  k%d: 1\n"),
		"tagged nested":                numbered("k%d: !!map\n  a: 1\n"),
		"anchors after keys":           numbered("key_%[1]d: &a%[1]d\n") + "z: 1\n",
		"anchors after entries":        items("- &a\n", 20000) + "- 1\n",
		"documents":                    items("---\na: 1\n", 1000),
		"tags at the limit":            "a: " + items("!!str ", maxChain) + "1",
		"nested flows":                 "a: " + items("[", 1000) + items("]", 1000),
		"nested sequences":             items("- ", 1000) + "a",
		"null items":                   items("-\n", n),
		"tagged items":                 items("- !!str a\n", n),
		"anchored items":               items("- &b a\n", n),
		"many entries":                 items("- [a]\n", 20000),
		"flow values":                  items("- [a]\n", n),
		"tags in a flow":               "a: [" + items("!!str a, ", n) + "b]\n",
		"comments":                     items("# [\n", n),
		"marks in a string":            "a: '" + items("!", 200_000) + "'\n",
		"quoted":                       "a: \"" + items("[", n) + "\"\n",
		"plain":                        "a: b" + items("[", n) + "\n",
		"block scalar":                 "a: |\n  " + items("[", n) + "\n",
		"long value":                   "a: " + key + "\n",
		"key before siblings":          key + ": 1\n" + keys(2000),
		"tagged key before siblings":   "!!str " + key + ": 1\n" + keys(2000),
		"explicit key before siblings": "? " + key + "\n: 1\n" + keys(2000),
		"key before entries":           "- " + key + ": 1\n" + items("- 1\n", 2000),
		"flow key and siblings":        "{" + key + ": 1, " + items("a, ", 2000) + "b: 2}",
		"closed flow and items":        "[{" + key + ": 1}, " + items("1, ", 2000) + "2]",
		"key in an earlier doc":        key + ": 1\n---\n" + items("- 1\n", 2000),
	}
	for name, text := range accepted {
		t.Run(name, func(t *testing.T) {
			if Parse(text) == nil {
				t.Error("did not parse")
			}
		})
	}
}

func keys(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "a%d: 1\n", i)
	}
	return b.String()
}
