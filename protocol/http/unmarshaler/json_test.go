package unmarshaler

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/scenarigo/scenarigo/assert"
)

func TestJSON_Unmarshal_BigInt(t *testing.T) {
	in := 8608570626085064778
	b := fmt.Appendf(nil, `{"id": %d}`, in)
	var v any
	um := &jsonUnmarshaler{}
	if err := um.Unmarshal(b, &v); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expect map[string]interface{} but got %T", v)
	}
	out, ok := m["id"]
	if !ok {
		t.Fatal("id not found")
	}

	if err := assert.Equal(in).Assert(out); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if got, expect := jsonString(t, out), jsonString(t, in); got != expect {
		t.Errorf("expect %s but got %s", expect, got)
	}
}

func jsonString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %s", err)
	}
	return string(b)
}
