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

func TestReplaceOutput_KeepScenarigoHeaders(t *testing.T) {
	str := `
request:
  method: GET
  url: http://[::]:35233/echo
  header:
    Scenarigo-Scenario-Filepath:
    - testdata%2Fscenarios%2Ftest.yaml
    Scenarigo-Scenario-Title:
    - test%20scenario
    Scenarigo-Step-Full-Name:
    - testdata%2Fscenarios%2Ftest.yaml%2Ftest_scenario%2FGET_%2Fecho
    User-Agent:
    - scenarigo/v1.0.0
`
	t.Run("without option", func(t *testing.T) {
		expect := `
request:
  method: GET
  url: http://[::]:12345/echo
  header:
    User-Agent:
    - scenarigo/v1.0.0
`
		if diff := cmp.Diff(expect, ReplaceOutput(str)); diff != "" {
			t.Errorf("differs (-want +got):\n%s", diff)
		}
	})
	t.Run("with KeepScenarigoHeaders", func(t *testing.T) {
		expect := `
request:
  method: GET
  url: http://[::]:12345/echo
  header:
    Scenarigo-Scenario-Filepath:
    - testdata%2Fscenarios%2Ftest.yaml
    Scenarigo-Scenario-Title:
    - test%20scenario
    Scenarigo-Step-Full-Name:
    - testdata%2Fscenarios%2Ftest.yaml%2Ftest_scenario%2FGET_%2Fecho
    User-Agent:
    - scenarigo/v1.0.0
`
		if diff := cmp.Diff(expect, ReplaceOutput(str, KeepScenarigoHeaders())); diff != "" {
			t.Errorf("differs (-want +got):\n%s", diff)
		}
	})
}

func TestRemoveScenarigoHeaders(t *testing.T) {
	tests := map[string]struct {
		input  string
		expect string
	}{
		"removes all scenarigo headers": {
			input: `header:
  Scenarigo-Scenario-Filepath:
  - testdata%2Ftest.yaml
  Scenarigo-Scenario-Title:
  - test
  Scenarigo-Step-Full-Name:
  - testdata%2Ftest.yaml%2Ftest%2Fstep
  User-Agent:
  - scenarigo/v1.0.0`,
			expect: `header:
  User-Agent:
  - scenarigo/v1.0.0`,
		},
		"keeps other headers": {
			input: `header:
  Content-Type:
  - application/json
  Authorization:
  - Bearer token`,
			expect: `header:
  Content-Type:
  - application/json
  Authorization:
  - Bearer token`,
		},
		"handles empty input": {
			input:  "",
			expect: "",
		},
		"handles input without headers": {
			input:  "no headers here",
			expect: "no headers here",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(test.expect, RemoveScenarigoHeaders(test.input)); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}
