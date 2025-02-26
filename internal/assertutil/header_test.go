package assertutil

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/scenarigo/scenarigo/context"
)

func Test_BuildHeaderAssertion(t *testing.T) {
	tests := map[string]struct {
		in string
		ok map[string][]string
		ng map[string][]string
	}{
		"simple": {
			in: `
foo:
- bar
`,
			ok: map[string][]string{
				"foo": {
					"bar",
				},
			},
			ng: map[string][]string{
				"foo": {
					"baz",
				},
			},
		},
		"assert function": {
			in: `
foo:
- '{{assert.notZero}}'
`,
			ok: map[string][]string{
				"foo": {
					"bar",
				},
			},
			ng: map[string][]string{
				"foo": {
					"",
				},
			},
		},
		"not array": {
			in: `
foo: bar
`,
			ok: map[string][]string{
				"foo": {
					"bar",
				},
			},
			ng: map[string][]string{
				"foo": {
					"baz",
				},
			},
		},
		"not array (assert function)": {
			in: `
foo: '{{assert.notZero}}'
`,
			ok: map[string][]string{
				"foo": {
					"bar",
				},
			},
			ng: map[string][]string{
				"foo": {
					"",
				},
			},
		},
		"not array (with $)": {
			in: `
foo: '{{$ != ""}}'
`,
			ok: map[string][]string{
				"foo": {
					"bar",
				},
			},
			ng: map[string][]string{
				"foo": {
					"",
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var expect yaml.MapSlice
			if err := yaml.NewDecoder(strings.NewReader(test.in)).Decode(&expect); err != nil {
				t.Fatalf("failed to decode: %s", err)
			}
			assertion, err := BuildHeaderAssertion(context.FromT(t), expect)
			if err != nil {
				t.Fatalf("failed to build assertion: %s", err)
			}
			if test.ok != nil {
				if err := assertion.Assert(test.ok); err != nil {
					t.Errorf("unexpected error: %s", err)
				}
			}
			if err := assertion.Assert(test.ng); err == nil {
				t.Error("no error")
			}
		})
	}
}

func Test_BuildHeaderAssertion_Error(t *testing.T) {
	tests := map[string]struct {
		expect yaml.MapSlice
	}{
		"invalid key": {
			expect: yaml.MapSlice{
				yaml.MapItem{
					Key:   nil,
					Value: "value",
				},
			},
		},
		"unknown vars": {
			expect: yaml.MapSlice{
				yaml.MapItem{
					Key:   "key",
					Value: "{{vars.unknown}}",
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildHeaderAssertion(context.FromT(t), test.expect); err == nil {
				t.Error("no error")
			}
		})
	}
}

func Test_Stringify(t *testing.T) {
	tests := map[string]struct {
		in     interface{}
		expect interface{}
	}{
		"bool": {
			in:     true,
			expect: "true",
		},
		"int": {
			in:     123,
			expect: "123",
		},
		"int8": {
			in:     int8(123),
			expect: "123",
		},
		"int16": {
			in:     int16(123),
			expect: "123",
		},
		"int32": {
			in:     int32(123),
			expect: "123",
		},
		"int64": {
			in:     int64(123),
			expect: "123",
		},
		"uint8": {
			in:     uint8(123),
			expect: "123",
		},
		"uint16": {
			in:     uint16(123),
			expect: "123",
		},
		"uint32": {
			in:     uint32(123),
			expect: "123",
		},
		"uint64": {
			in:     uint64(123),
			expect: "123",
		},
		"nil": {
			in:     nil,
			expect: nil,
		},
		"float64": {
			in:     1.23,
			expect: 1.23,
		},
		"yaml.MapSlice": {
			in: yaml.MapSlice{
				yaml.MapItem{
					Key:   "key",
					Value: 123,
				},
			},
			expect: yaml.MapSlice{
				yaml.MapItem{
					Key:   "key",
					Value: "123",
				},
			},
		},
		"[]interface{}": {
			in:     []interface{}{123},
			expect: []interface{}{"123"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := stringify(test.in)
			if !reflect.DeepEqual(got, test.expect) {
				t.Fatalf("expect %+v but got %+v", test.expect, got)
			}
		})
	}
}
