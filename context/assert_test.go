package context_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/template"
)

func TestLeftArrowFunc(t *testing.T) {
	tests := map[string]struct {
		yaml string
		ok   interface{}
		ng   interface{}
	}{
		"simple": {
			yaml: `'{{f <-}}: 1'`,
			ok:   []int{0, 1},
			ng:   []int{2, 3},
		},
		"nest": {
			yaml: strconv.Quote(strings.Trim(`
{{f <-}}:
  ids: |-
    {{f <-}}: 1
`, "\n")),
			ok: []interface{}{
				map[string]interface{}{
					"ids": []int{0, 1},
				},
			},
			ng: []interface{}{
				map[string]interface{}{
					"ids": []int{2, 3},
				},
			},
		},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			var i interface{}
			if err := yaml.Unmarshal([]byte(tc.yaml), &i); err != nil {
				t.Fatalf("failed to unmarshal: %s", err)
			}
			v, err := template.ExecuteWithArgs(context.FromT(t), i, map[string]interface{}{
				"f": context.ContainsAssertion(),
			})
			if err != nil {
				t.Fatalf("failed to execute: %s", err)
			}
			assertion := assert.Build(v)
			if err := assertion.Assert(tc.ok); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if err := assertion.Assert(tc.ng); err == nil {
				t.Errorf("expected error but no error")
			}
		})
	}
}
