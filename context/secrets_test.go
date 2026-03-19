package context

import (
	"testing"

	"github.com/zoncoen/query-go"
)

func TestSecrets(t *testing.T) {
	var v Secrets
	v1 := v.Append(map[string]int{"1": 1})
	v2 := v1.Append(map[string]int{"2": 2})
	v3 := v2.Append(map[string]int{"3": 3})

	checkSecrets(t, v1, ".1", 1, false)
	checkSecrets(t, v1, ".2", nil, true)
	checkSecrets(t, v1, ".3", nil, true)

	checkSecrets(t, v2, ".1", 1, false)
	checkSecrets(t, v2, ".2", 2, false)
	checkSecrets(t, v2, ".3", nil, true)

	checkSecrets(t, v3, ".1", 1, false)
	checkSecrets(t, v3, ".2", 2, false)
	checkSecrets(t, v3, ".3", 3, false)
}

func checkSecrets(t *testing.T, secrets *Secrets, s string, expect any, expectErr bool) {
	t.Helper()
	q, err := query.ParseString(s)
	if err != nil {
		t.Fatalf("failed to parse: %s", err)
	}
	got, err := q.Extract(secrets)
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
