package context

import (
	"context"
	"errors"
	"testing"

	query "github.com/zoncoen/query-go/v2"

	"github.com/scenarigo/scenarigo/internal/queryutil"
)

func TestVars(t *testing.T) {
	var v Vars
	v1 := v.Append(map[string]int{"1": 1})
	v2 := v1.Append(map[string]int{"2": 2})
	v3 := v2.Append(map[string]int{"3": 3})

	checkVars(t, v1, ".1", 1, false)
	checkVars(t, v1, ".2", nil, true)
	checkVars(t, v1, ".3", nil, true)

	checkVars(t, v2, ".1", 1, false)
	checkVars(t, v2, ".2", 2, false)
	checkVars(t, v2, ".3", nil, true)

	checkVars(t, v3, ".1", 1, false)
	checkVars(t, v3, ".2", 2, false)
	checkVars(t, v3, ".3", 3, false)
}

func checkVars(t *testing.T, vars Vars, s string, expect any, expectErr bool) {
	t.Helper()
	q, err := query.ParseString(s)
	if err != nil {
		t.Fatalf("failed to parse: %s", err)
	}
	got, err := q.Extract(context.Background(), vars)
	if expect != got {
		t.Errorf("expected %v, got %v", expect, got)
	}
	if !expectErr && err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if expectErr && err == nil {
		t.Error("no error")
	}
}

type failingKey struct{}

func (failingKey) ExtractByKey(context.Context, string) (any, error) {
	return nil, errors.New("lookup failed")
}

func TestVars_ExtractByKey_Failure(t *testing.T) {
	var v Vars
	v = v.Append(map[string]int{"k": 1}).Append(failingKey{})
	_, err := v.ExtractByKey(context.Background(), "k")
	if err == nil || errors.Is(err, query.ErrNotFound) {
		t.Fatalf("expected the failure to be reported but got %v", err)
	}
}

func TestVars_CaseInsensitiveReachesTheSubQuery(t *testing.T) {
	// Vars looks the key up in each appended value with a sub-query. That
	// sub-query has to carry the options of the lookup it is part of, or a
	// case-insensitive reference stops being case-insensitive the moment it
	// reaches the variables.
	vars := Vars{}.Append(map[string]any{"Foo": "FOO"})
	q := query.New(append(queryutil.Options(), query.CaseInsensitive())...).Key("vars").Key("foo")
	got, err := q.Extract(context.Background(), map[string]any{"vars": vars})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if expect := "FOO"; got != expect {
		t.Errorf("expected %q but got %v", expect, got)
	}
}
