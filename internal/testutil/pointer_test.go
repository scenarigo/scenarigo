package testutil

import "testing"

func TestToPtr(t *testing.T) {
	v1 := "aaa"
	if p := ToPtr(v1); *p != v1 {
		t.Fatalf("expect %q but got %q", v1, *p)
	}
}
