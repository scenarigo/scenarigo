package lsp

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml/ast"

	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
)

// parseDocument returns the view and the body of the only document of text.
func parseDocument(t *testing.T, text string) (*yamlView, ast.Node) {
	t.Helper()
	for v, d := range yamlDocuments(parseFile(t, text)) {
		return v, d.Body
	}
	t.Fatalf("no document in %q", text)
	return nil, nil
}

func parseFile(t *testing.T, text string) *ast.File {
	t.Helper()
	parsed := yamlutil.Parse(text)
	if parsed == nil || parsed.File == nil {
		t.Fatalf("failed to parse %q", text)
	}
	return parsed.File
}

func entryKeys(entries []*ast.MappingValueNode) []string {
	var keys []string
	for _, mv := range entries {
		keys = append(keys, keyName(mv.Key))
	}
	return keys
}

func TestYAMLView_MergeKeysExpandEachMappingOnce(t *testing.T) {
	tests := map[string]string{
		"a cycle":             "vars: &a\n  x: 1\n  <<: *a\n",
		"a cycle with fanout": "vars: &a\n  x: 1\n  <<: [*a, *a, *a, *a, *a, *a]\n",
	}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			done := make(chan []string, 1)
			go func() {
				v, body := parseDocument(t, text)
				done <- entryKeys(v.entries(v.entryValue(v.entries(body), "vars")))
			}()
			select {
			case keys := <-done:
				if !slices.Equal(keys, []string{"x"}) {
					t.Fatalf("unexpected keys: %v", keys)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("merge keys did not finish")
			}
		})
	}
}

func TestYAMLView_MergesFollowLongChains(t *testing.T) {
	var text strings.Builder
	text.WriteString("a0: &a0 {x: 1}\n")
	depth := maxAliasDepth * 2
	for i := 1; i < depth; i++ {
		fmt.Fprintf(&text, "a%d: &a%d {<<: *a%d}\n", i, i, i-1)
	}
	fmt.Fprintf(&text, "top: {<<: *a%d}\n", depth-1)
	v, body := parseDocument(t, text.String())
	keys := entryKeys(v.entries(v.entryValue(v.entries(body), "top")))
	if !slices.Equal(keys, []string{"x"}) {
		t.Fatalf("unexpected keys: %v", keys)
	}
}

func TestYAMLView_AnchorsCarryOverToLaterDocuments(t *testing.T) {
	// schema/load.go: one decoder reads the documents of a file.
	text := "base: &base {a: 1}\nvars: *base\n---\nvars: *base\n---\nbase: &base {b: 1}\nvars: *base\n"
	var got [][]string
	for v, d := range yamlDocuments(parseFile(t, text)) {
		got = append(got, entryKeys(v.entries(v.entryValue(v.entries(d.Body), "vars"))))
	}
	if want := [][]string{{"a"}, {"a"}, {"b"}}; !slices.EqualFunc(got, want, slices.Equal[[]string]) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestYAMLView_TheKeyWrittenLastWins(t *testing.T) {
	v, body := parseDocument(t, "base: &base {a: merged, b: merged, c: merged}\nm:\n  a: own\n  <<: *base\n  c: own\n")
	entries := v.entries(v.entryValue(v.entries(body), "m"))
	for key, want := range map[string]string{"a": "merged", "b": "merged", "c": "own"} {
		if got := v.stringValue(v.entryValue(entries, key)); got != want {
			t.Errorf("%s: got %q, want %q", key, got, want)
		}
	}
}

func TestYAMLView_TheMergeWrittenLastWins(t *testing.T) {
	v, body := parseDocument(t, "a: &a {k: first}\nb: &b {k: second}\nm:\n  <<: [*a, *b]\n")
	entries := v.entries(v.entryValue(v.entries(body), "m"))
	if got := v.stringValue(v.entryValue(entries, "k")); got != "second" {
		t.Errorf("got %q", got)
	}
}

func TestYAMLView_KeyNames(t *testing.T) {
	v, body := parseDocument(t, "\"quoted\": 1\n'single': 2\ncommented: # note\n  a: 1\n")
	got := entryKeys(v.entries(body))
	slices.Sort(got)
	if want := []string{"commented", "quoted", "single"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestYAMLView_StringValue(t *testing.T) {
	for text, want := range map[string]string{
		"v: a":       "a",
		"v: 'a'":     "a",
		"v: 123":     "123",
		"v: 1.5":     "1.5",
		"v: true":    "true",
		"v: >-\n  a": "a",
		"v: null":    "",
		"v: [a]":     "",
	} {
		v, body := parseDocument(t, text+"\n")
		if got := v.stringValue(v.entryValue(v.entries(body), "v")); got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}
