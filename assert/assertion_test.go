package assert

import (
	"strconv"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/template"
)

func TestBuild(t *testing.T) {
	var str = `
deps:
- name: scenarigo
  version:
    major: 1
    minor: 2
    patch: 3
  tags:
    - go
    - test`
	var in interface{}
	if err := yaml.NewDecoder(strings.NewReader(str), yaml.UseOrderedMap()).Decode(&in); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	qs := []string{
		".deps[0].name",
		".deps[0].version.major",
		".deps[0].version.minor",
		".deps[0].version.patch",
		".deps[0].tags[0]",
		".deps[0].tags[1]",
	}
	assertion := Build(in)

	type info struct {
		Deps []map[string]interface{} `yaml:"deps"`
	}

	t.Run("no assertion", func(t *testing.T) {
		ctx := context.FromT(t)
		assertion := Build(nil)
		v := info{}
		if err := assertion.Assert(ctx, v); err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	})
	t.Run("ok", func(t *testing.T) {
		ctx := context.FromT(t)
		v := info{
			Deps: []map[string]interface{}{
				{
					"name": "scenarigo",
					"version": map[string]int{
						"major": 1,
						"minor": 2,
						"patch": 3,
					},
					"tags": []string{"go", "test"},
				},
			},
		}
		if err := assertion.Assert(ctx, v); err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	})
	t.Run("ng", func(t *testing.T) {
		ctx := context.FromT(t)
		v := info{
			Deps: []map[string]interface{}{
				{
					"name": "Ruby on Rails",
					"version": map[string]int{
						"major": 2,
						"minor": 3,
						"patch": 4,
					},
					"tags": []string{"ruby", "http"},
				},
			},
		}
		err := assertion.Assert(ctx, v)
		if err == nil {
			t.Fatalf("expected error but no error")
		}
		errs := err.(*Error).Errors
		if got, expect := len(errs), len(qs); got != expect {
			t.Fatalf("expected %d but got %d", expect, got)
		}
		for i, e := range errs {
			q := qs[i]
			if !strings.Contains(e.Error(), q) {
				t.Errorf(`"%s" does not contain "%s"`, e.Error(), q)
			}
		}
	})
	t.Run("assert nil", func(t *testing.T) {
		ctx := context.FromT(t)
		err := assertion.Assert(ctx, nil)
		if err == nil {
			t.Fatalf("expected error but no error")
		}
		errs := err.(*Error).Errors
		if got, expect := len(errs), len(qs); got != expect {
			t.Fatalf("expected %d but got %d", expect, got)
		}
		for i, e := range errs {
			q := qs[i]
			if !strings.Contains(e.Error(), q) {
				t.Errorf(`"%s" does not contain "%s"`, e.Error(), q)
			}
		}
	})
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
			ctx := context.FromT(t)
			var i interface{}
			if err := yaml.Unmarshal([]byte(tc.yaml), &i); err != nil {
				t.Fatalf("failed to unmarshal: %s", err)
			}
			v, err := template.ExecuteWithArgs(ctx, i, map[string]interface{}{
				"f": assertions["contains"],
			})
			if err != nil {
				t.Fatalf("failed to execute: %s", err)
			}
			assertion := Build(v)
			if err := assertion.Assert(ctx, tc.ok); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if err := assertion.Assert(ctx, tc.ng); err == nil {
				t.Errorf("expected error but no error")
			}
		})
	}
}
