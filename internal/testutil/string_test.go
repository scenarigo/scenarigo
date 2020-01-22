package testutil

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestResetDuration(t *testing.T) {
	str := `
=== RUN   test.yaml
--- PASS: test.yaml (1.23s)
PASS
ok  	test.yaml	1.234s
`
	expect := `
=== RUN   test.yaml
--- PASS: test.yaml (0.00s)
PASS
ok  	test.yaml	0.000s
`
	if diff := cmp.Diff(expect, ResetDuration(str)); diff != "" {
		t.Errorf("differs (-want +got):\n%s", diff)
	}
}
