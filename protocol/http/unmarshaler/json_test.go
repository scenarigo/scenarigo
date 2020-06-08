package unmarshaler

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/zoncoen/query-go"

	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
)

func TestJSON_Unmarshal_BigInt(t *testing.T) {
	in := 8608570626085064778
	b := []byte(fmt.Sprintf(`{"id": %d}`, in))
	var v interface{}
	um := &jsonUnmarshaler{}
	if err := um.Unmarshal(b, &v); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("expect map[string]interface{} but got %T", v)
	}
	out, ok := m["id"]
	if !ok {
		t.Fatal("id not found")
	}

	ctx := context.FromT(t)

	if err := assert.Equal(query.New(), in).Assert(ctx, out); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	expect := jsonString(t, in)
	got := jsonString(t, out)
	if got != expect {
		t.Errorf("expect %s but got %s", expect, got)
	}
}

func jsonString(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %s", err)
	}
	return string(b)
}
