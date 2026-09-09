package yamlutil

import (
	"fmt"
	"testing"
)

func TestGetCursorContext_StepKeys(t *testing.T) {
	text := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    "
	doc := Parse(text)
	if doc == nil {
		t.Fatal("parse failed")
	}

	// Line 5 (0-based), char 4 — inside a step, should be key context
	ctx := doc.GetCursorContext(5, 4)
	fmt.Printf("Type: %d (0=unknown, 1=key, 2=value)\n", ctx.Type)
	fmt.Printf("Path: %v\n", ctx.Path)
	fmt.Printf("ParentKeys: %v\n", ctx.ParentKeys)
	fmt.Printf("PartialKey: %q\n", ctx.PartialKey)

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
	fmt.Printf("Type: %d\n", ctx.Type)
	fmt.Printf("Path: %v\n", ctx.Path)
	fmt.Printf("PartialValue: %q\n", ctx.PartialValue)

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
