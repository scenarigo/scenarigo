package context

import (
	"testing"

	"github.com/zoncoen/query-go"
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
	got, err := q.Extract(vars)
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
