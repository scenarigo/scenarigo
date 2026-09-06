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
