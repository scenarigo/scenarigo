package queryutil

import (
	"context"
	"errors"
	"testing"

	query "github.com/zoncoen/query-go/v2"
)

type failingKey struct{}

func (failingKey) ExtractByKey(context.Context, string) (any, error) {
	return nil, errors.New("lookup failed")
}

func TestExtractFirst(t *testing.T) {
	ctx := context.Background()
	q := New().Key("k")

	t.Run("the first target that has the value wins", func(t *testing.T) {
		got, err := ExtractFirst(ctx, q, nil, map[string]int{"other": 0}, map[string]int{"k": 1}, map[string]int{"k": 2})
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if got != 1 {
			t.Errorf("expected 1 but got %v", got)
		}
	})

	t.Run("no target has the value", func(t *testing.T) {
		if _, err := ExtractFirst(ctx, q, nil, map[string]int{"other": 0}); !errors.Is(err, query.ErrNotFound) {
			t.Fatalf("expected ErrNotFound but got %v", err)
		}
	})

	t.Run("a failure stops the lookup", func(t *testing.T) {
		_, err := ExtractFirst(ctx, q, failingKey{}, map[string]int{"k": 1})
		if err == nil || errors.Is(err, query.ErrNotFound) {
			t.Fatalf("expected the failure to be reported but got %v", err)
		}
	})
}

// caseInsensitiveTarget is reachable only when the sub-query an extractor runs
// keeps the case-insensitivity of the lookup it is part of.
type caseInsensitiveTarget struct {
	Value string
}

type nestingExtractor struct {
	inner any
}

func (e *nestingExtractor) ExtractByKey(ctx context.Context, key string) (any, error) {
	return NewFromContext(ctx).Key(key).Extract(ctx, e.inner)
}

func TestNewFromContext(t *testing.T) {
	t.Run("a sub-query inherits the options of the lookup", func(t *testing.T) {
		target := &nestingExtractor{inner: caseInsensitiveTarget{Value: "found"}}
		// The outer lookup is case-insensitive; the field it ends at is only
		// reachable if the sub-query the extractor runs is too.
		got, err := query.New(append(Options(), query.CaseInsensitive())...).
			Key("value").
			Extract(context.Background(), target)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if got != "found" {
			t.Errorf("expected %q but got %v", "found", got)
		}
	})
	t.Run("outside an extraction it falls back to the process options", func(t *testing.T) {
		// Reach the field by its yaml tag rather than its name, which only the
		// process-wide ExtractByStructTag option can do. A plain query.New()
		// would not find it, so the fallback is what this asserts.
		q := NewFromContext(context.Background()).Key("alias")
		got, err := q.Extract(context.Background(), taggedTarget{Value: "found"})
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if got != "found" {
			t.Errorf("expected %q but got %v", "found", got)
		}
	})
}

// taggedTarget is reachable by its yaml tag only through the process-wide
// ExtractByStructTag option.
type taggedTarget struct {
	Value string `yaml:"alias"`
}
