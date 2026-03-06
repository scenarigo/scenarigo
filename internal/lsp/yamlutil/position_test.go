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
