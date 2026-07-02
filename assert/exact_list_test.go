package assert

import (
	"context"
	"strings"
	"testing"
)

func TestExactList(t *testing.T) {
	ctx := context.Background()

	t.Run("extra element fails", func(t *testing.T) {
		err := MustBuild(ctx, []any{"a", "b"}, ExactList()).Assert([]any{"a", "b", "c"})
		if err == nil {
			t.Fatal("expected error for an extra element but got nil")
		}
		if got := err.Error(); !strings.Contains(got, "length: expected 2 but got 3") {
			t.Errorf("expected a length error but got %q", got)
		}
	})

	t.Run("exact match passes", func(t *testing.T) {
		if err := MustBuild(ctx, []any{"a", "b"}, ExactList()).Assert([]any{"a", "b"}); err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	})

	t.Run("count and element mismatches are reported together", func(t *testing.T) {
		err := MustBuild(ctx, []any{"x", "y"}, ExactList()).Assert([]any{"a", "b", "c"})
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		got := err.Error()
		if !strings.Contains(got, "length: expected 2 but got 3") {
			t.Errorf("expected the count mismatch to be reported but got %q", got)
		}
		if !strings.Contains(got, `[0]: expected "x" but got "a"`) {
			t.Errorf("expected the element mismatch to be reported too but got %q", got)
		}
	})

	t.Run("nested lists are exact too", func(t *testing.T) {
		expect := map[string]any{"tags": []any{"a"}}
		err := MustBuild(ctx, expect, ExactList()).Assert(map[string]any{"tags": []any{"a", "b"}})
		if err == nil {
			t.Fatal("expected error for an extra nested element but got nil")
		}
	})

	t.Run("default keeps prefix matching", func(t *testing.T) {
		if err := MustBuild(ctx, []any{"a", "b"}).Assert([]any{"a", "b", "c"}); err != nil {
			t.Errorf("expected the default prefix match to pass but got: %s", err)
		}
	})
}
