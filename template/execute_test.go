package template

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zoncoen/yaml"
)

func TestExecute(t *testing.T) {
	var iface interface{} = `{{"test"}}`
	tests := map[string]struct {
		in       interface{}
		expected interface{}
		vars     interface{}
	}{
		"string": {
			in:       "test",
			expected: "test",
		},
		"template string": {
			in:       "{{test}}",
			expected: "test",
			vars: map[string]string{
				"test": "test",
			},
		},
		"integer": {
			in:       1,
			expected: 1,
		},
		"nil": {
			in:       nil,
			expected: nil,
		},
		"nil map": {
			in:       map[interface{}]interface{}(nil),
			expected: map[interface{}]interface{}(nil),
		},
		"nil slice": {
			in:       []interface{}(nil),
			expected: []interface{}(nil),
		},
		"map[string]string": {
			in: map[string]string{
				"env": `{{"test"}}`,
			},
			expected: map[string]string{
				"env": "test",
			},
		},
		"map[string]interface{}": {
			in: map[string]interface{}{
				"env":     `{{"test"}}`,
				"version": "{{1}}",
				"nil":     nil,
			},
			expected: map[string]interface{}{
				"env":     "test",
				"version": 1,
				"nil":     nil,
			},
		},
		"map[string][]string": {
			in: map[string][]string{
				"env": {`{{"test"}}`},
			},
			expected: map[string][]string{
				"env": {"test"},
			},
		},
		"[]string": {
			in:       []string{`{{"one"}}`, "two", `{{"three"}}`},
			expected: []string{"one", "two", "three"},
		},
		"[]interface{}": {
			in:       []interface{}{`{{"one"}}`, `{{1}}`, nil},
			expected: []interface{}{"one", 1, nil},
		},
		"yaml.MapSlice": {
			in: yaml.MapSlice{
				yaml.MapItem{
					Key:   "id",
					Value: 100,
				},
				yaml.MapItem{
					Key:   "name",
					Value: `{{"Bob"}}`,
				},
			},
			expected: yaml.MapSlice{
				yaml.MapItem{
					Key:   "id",
					Value: 100,
				},
				yaml.MapItem{
					Key:   "name",
					Value: "Bob",
				},
			},
		},
		"yaml.MapSlice (Value is nil)": {
			in: yaml.MapSlice{
				yaml.MapItem{
					Key:   "key",
					Value: nil,
				},
			},
			expected: yaml.MapSlice{
				yaml.MapItem{
					Key:   "key",
					Value: nil,
				},
			},
		},
		"pointer to interface{}": {
			in:       &iface,
			expected: "test",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			got, err := Execute(test.in, test.vars)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(test.expected, got); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
		})
	}
}
