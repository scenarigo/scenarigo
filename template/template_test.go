package template

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNew(t *testing.T) {
	tests := map[string]struct {
		str         string
		expectError bool
	}{
		"success": {
			str: `{{a.b[0]}}`,
		},
		"failed": {
			str:         `{{}`,
			expectError: true,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			_, err := New(test.str)
			if !test.expectError && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if test.expectError && err == nil {
				t.Fatal("expected error but got no error")
			}
		})
	}
}

func TestTemplate_Execute(t *testing.T) {
	tests := map[string]struct {
		str         string
		data        interface{}
		expect      interface{}
		expectError bool
	}{
		"no parameter": {
			str:    "1",
			expect: "1",
		},
		"empty string": {
			str:    "",
			expect: "",
		},
		"empty parameter": {
			str:    "{{}}",
			expect: "",
		},
		"empty parameter with string": {
			str:    "prefix-{{}}-suffix",
			expect: "prefix--suffix",
		},
		"string": {
			str:    `{{"foo"}}`,
			expect: "foo",
		},
		"integer": {
			str:    "{{1}}",
			expect: 1,
		},
		"add": {
			str:    `foo-{{ "bar" + "-" + "baz" }}`,
			expect: "foo-bar-baz",
		},
		"query from data": {
			str: "{{a.b[1]}}",
			data: map[string]map[string][]string{
				"a": map[string][]string{
					"b": []string{"ng", "ok"},
				},
			},
			expect: "ok",
		},
		"function call": {
			str: `{{f("ok")}}`,
			data: map[string]func(string) string{
				"f": func(s string) string { return s }},
			expect: "ok",
		},
		"function call with YAML arg": {
			str: strings.Trim(`
{{echo <-}}:
  message: '{{message}}'
`, "\n"),
			data: map[string]interface{}{
				"echo":    &echoFunc{},
				"message": "hello",
			},
			expect: "hello",
		},
		"function call with YAML arg (nest)": {
			str: strings.Trim(`
{{echo <-}}:
  message: |
    {{echo <-}}:
      message: '{{message}}'
`, "\n"),
			data: map[string]interface{}{
				"echo":    &echoFunc{},
				"message": "hello",
			},
			expect: "hello",
		},
		"function call with YAML arg (complex)": {
			str: strings.Trim(`
{{join <-}}:
  prefix: preout-
  text: |-
    {{join <-}}:
      prefix: prein-
      text: '{{text}}'
      suffix: -sufin
  suffix: -sufout
`, "\n"),
			data: map[string]interface{}{
				"join": &joinFunc{},
				"text": "test",
			},
			expect: "preout-prein-test-sufin-sufout",
		},
		"not found": {
			str:         "{{a.b[1]}}",
			expectError: true,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			tmpl, err := New(test.str)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			i, err := tmpl.Execute(test.data)
			if !test.expectError && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if test.expectError && err == nil {
				t.Fatal("expected error but got no error")
			}
			if diff := cmp.Diff(test.expect, i); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

var _ Func = &echoFunc{}

type echoFunc struct{}

type echoArg struct {
	Message string `yaml:"message"`
}

func (_ *echoFunc) Exec(in interface{}) (interface{}, error) {
	arg, ok := in.(echoArg)
	if !ok {
		return nil, errors.New("arg must be a echoArg")
	}
	return arg.Message, nil
}

func (_ *echoFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var arg echoArg
	if err := unmarshal(&arg); err != nil {
		return nil, err
	}
	return arg, nil
}

type joinFunc struct{}

type joinArg struct {
	Prefix string `yaml:"prefix"`
	Text   string `yaml:"text"`
	Suffix string `yaml:"suffix"`
}

func (_ *joinFunc) Exec(in interface{}) (interface{}, error) {
	arg, ok := in.(*joinArg)
	if !ok {
		return nil, errors.New("arg must be a joinArg")
	}
	return arg.Prefix + arg.Text + arg.Suffix, nil
}

func (_ *joinFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var arg joinArg
	if err := unmarshal(&arg); err != nil {
		return nil, err
	}
	return &arg, nil
}
