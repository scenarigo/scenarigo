package template

import (
	"errors"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
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
				"a": {
					"b": {"ng", "ok"},
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
		"call function that have argument required cast": {
			str: `{{f(1, 2, 3, 4, 5)}}`,
			data: map[string]func(int, int8, int16, int32, int64) int{
				"f": func(a0 int, a1 int8, a2 int16, a3 int32, a4 int64) int {
					return a0 + int(a1) + int(a2) + int(a3) + int(a4)
				}},
			expect: 15,
		},
		"left arrow func": {
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
		"left arrow func (nest)": {
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
				"call": &callFunc{},
				"f":    func(s string) string { return s },
				"text": "test",
			},
			expect: "preout-prein-test-sufin-sufout",
		},
		"left arrow func with the arg which contains non-string variable": {
			str: strings.Trim(`
{{echo <-}}:
  message: |-
    {{echo <-}}:
      message: '{{message}}'
`, "\n"),
			data: map[string]interface{}{
				"echo":    &echoFunc{},
				"message": 0,
			},
			expect: "0",
		},
		"left arrow func (complex)": {
			str: strings.Trim(`
{{join <-}}:
  prefix: pre-
  text: |-
    {{call <-}}:
      f: '{{f}}'
      arg: '{{text}}'
  suffix: -suf
`, "\n"),
			data: map[string]interface{}{
				"join": &joinFunc{},
				"call": &callFunc{},
				"f":    func(s string) string { return s },
				"text": "test",
			},
			expect: "pre-test-suf",
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

func TestTemplate_ExecuteDirect(t *testing.T) {
	tests := map[string]struct {
		i           interface{}
		data        interface{}
		expect      interface{}
		expectError bool
	}{
		"not found by MapItem": {
			i: yaml.MapSlice{
				{
					Key:   "a",
					Value: "{{b}}",
				},
			},
			expectError: true,
		},
		"not found by struct": {
			i: struct {
				A string
			}{
				A: "{{b}}",
			},
			expectError: true,
		},
		"not found by struct with tag": {
			i: struct {
				A string `yaml:"a"`
			}{
				A: "{{b}}",
			},
			expectError: true,
		},
		"not found by map": {
			i: map[string]string{
				"a": "{{b}}",
			},
			expectError: true,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			i, err := Execute(test.i, test.data)
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

func (*echoFunc) Exec(in interface{}) (interface{}, error) {
	arg, ok := in.(echoArg)
	if !ok {
		return nil, errors.New("arg must be a echoArg")
	}
	return arg.Message, nil
}

func (*echoFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
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

func (*joinFunc) Exec(in interface{}) (interface{}, error) {
	arg, ok := in.(*joinArg)
	if !ok {
		return nil, errors.New("arg must be a joinArg")
	}
	return arg.Prefix + arg.Text + arg.Suffix, nil
}

func (*joinFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var arg joinArg
	if err := unmarshal(&arg); err != nil {
		return nil, err
	}
	return &arg, nil
}

type callFunc struct{}

type callArg struct {
	F   interface{} `yaml:"f"`
	Arg string      `yaml:"arg"`
}

func (*callFunc) Exec(in interface{}) (interface{}, error) {
	arg, ok := in.(*callArg)
	if !ok {
		return nil, errors.New("arg must be a callArg")
	}
	f, ok := arg.F.(func(string) string)
	if !ok {
		return nil, errors.New("arg.f must be a func(string) string")
	}
	return f(arg.Arg), nil
}

func (*callFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var arg callArg
	if err := unmarshal(&arg); err != nil {
		return nil, err
	}
	return &arg, nil
}
