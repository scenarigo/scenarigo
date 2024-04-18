package ptr

import "testing"

func TestTo(t *testing.T) {
	p := To(1)
	if got, expect := *p, 1; got != expect {
		t.Errorf("got %v, expected %v", got, expect)
	}
}
