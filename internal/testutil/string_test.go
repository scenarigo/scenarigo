package testutil

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/version"
)

func TestReplaceOutput(t *testing.T) {
	str := fmt.Sprintf(`
=== RUN   test.yaml
--- FAIL: test.yaml (1.23s)
    request:
      method: GET
      url: http://[::]:35233/echo
      header:
        User-Agent:
        - scenarigo/%s
        Date:
        - Tue, 10 Nov 2009 23:00:00 GMT
    elapsed time: 0.123456 sec
       6 |     method: GET
       7 |     url: "http://{{env.TEST_HTTP_ADDR}}/echo"
       8 |   expect:
    >  9 |     code: OK
                     ^
    expected OK but got Internal Server Error
FAIL
FAIL    test.yaml      1.234s
FAIL
`, version.String())
	expect := `
=== RUN   test.yaml
--- FAIL: test.yaml (0.00s)
    request:
      method: GET
      url: http://[::]:12345/echo
      header:
        User-Agent:
        - scenarigo/v1.0.0
        Date:
        - Mon, 01 Jan 0001 00:00:00 GMT
    elapsed time: 0.000000 sec
       6 |     method: GET
       7 |     url: "http://{{env.TEST_HTTP_ADDR}}/echo"
       8 |   expect:
    >  9 |     code: OK
                     ^
    expected OK but got Internal Server Error
FAIL
FAIL    test.yaml      0.000s
FAIL
`
	if diff := cmp.Diff(expect, ReplaceOutput(str)); diff != "" {
		t.Errorf("differs (-want +got):\n%s", diff)
	}
}
