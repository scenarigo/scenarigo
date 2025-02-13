package context

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/internal/testutil"
	"github.com/scenarigo/scenarigo/template"
)

func TestAssertions(t *testing.T) {
	executor := func(r testutil.Reporter, decode func(interface{})) func(testutil.Reporter, interface{}) error {
		return func(r testutil.Reporter, v interface{}) error {
			var i interface{}
			decode(&i)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			return assert.MustBuild(ctx, i, assert.FromTemplate(map[string]interface{}{
				"assert": &assertions{ctx},
			})).Assert(v)
		}
	}
	testutil.RunParameterizedTests(
		t, executor,
		"testdata/assertion/and.yaml",
		"testdata/assertion/or.yaml",
		"testdata/assertion/contains.yaml",
	)
}

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
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			v, err := template.Execute(ctx, i, map[string]interface{}{
				"f": &leftArrowFunc{
					ctx: ctx,
					f:   buildArg(context.Background(), assert.Contains),
				},
			})
			if err != nil {
				t.Fatalf("failed to execute: %s", err)
			}
			assertion := assert.MustBuild(context.Background(), v)
			if err := assertion.Assert(tc.ok); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if err := assertion.Assert(tc.ng); err == nil {
				t.Errorf("expected error but no error")
			}
		})
	}
}
