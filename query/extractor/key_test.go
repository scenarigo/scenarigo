package extractor

import (
	"reflect"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
)

type testStruct struct {
	A      string
	B      string       `yaml:"2"`
	Inline inlineStruct `yaml:",inline"`
}

type inlineStruct struct {
	C string
}

type testKeyExtractor struct {
	v interface{}
}

func (f *testKeyExtractor) ExtractByKey(_ string) (interface{}, bool) {
	if f.v != nil {
		return f.v, true
	}
	return nil, false
}

func TestKey_Extract(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		tests := map[string]struct {
			key    string
			v      interface{}
			expect interface{}
		}{
			"map[string]string": {
				key: "key",
				v: map[string]string{
					"key": "value",
				},
				expect: "value",
			},
			"map[interface{}]interface{}": {
				key: "key",
				v: map[interface{}]interface{}{
					0:     0,
					"key": 1,
				},
				expect: 1,
			},
			"struct": {
				key:    "a",
				v:      testStruct{A: "AAA"},
				expect: "AAA",
			},
			"struct pointer": {
				key:    "a",
				v:      &testStruct{A: "AAA"},
				expect: "AAA",
			},
			"struct field tag": {
				key:    "2",
				v:      testStruct{B: "BBB"},
				expect: "BBB",
			},
			"inline struct": {
				key:    "c",
				v:      testStruct{Inline: inlineStruct{C: "CCC"}},
				expect: "CCC",
			},
			"key extractor": {
				key:    "key",
				v:      &testKeyExtractor{v: "value"},
				expect: "value",
			},
			"ordered map": {
				key: "paramA",
				v: yaml.MapSlice{
					{
						Key:   "paramA",
						Value: "value",
					},
				},
				expect: "value",
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				e := Key(test.key)
				v, ok := e.Extract(reflect.ValueOf(test.v))
				if !ok {
					t.Fatal("not found")
				}
				if diff := cmp.Diff(test.expect, v.Interface()); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
			})
		}
	})
	t.Run("not found", func(t *testing.T) {
		tests := map[string]struct {
			key string
			v   interface{}
		}{
			"target is nil": {
				key: "key",
				v:   nil,
			},
			"key not found": {
				key: "key",
				v:   map[string]string{},
			},
			"field not found": {
				key: "A",
				v:   testStruct{},
			},
			"inline struct": {
				key: "inline",
				v:   testStruct{Inline: inlineStruct{C: "CCC"}},
			},
			"key extractor returns false": {
				key: "key",
				v:   &testKeyExtractor{},
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				e := Key(test.key)
				v, ok := e.Extract(reflect.ValueOf(test.v))
				if ok {
					t.Fatalf("unexpected value: %#v", v)
				}
			})
		}
	})
}
