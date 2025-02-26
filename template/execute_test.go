package template

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

func TestExecute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		type s struct {
			Str        string
			StrPtr     *string
			unexported string
		}
		var (
			tmpl              = `{{"test"}}`
			str               = "test"
			iface interface{} = `{{"test"}}`
		)
		tests := map[string]struct {
			in       interface{}
			expected interface{}
			vars     interface{}
		}{
			"string": {
				in:       "test",
				expected: "test",
			},
			"*string": {
				in:       &str,
				expected: &str,
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
			"map[string]*string": {
				in: map[string]*string{
					"env": &tmpl,
				},
				expected: map[string]*string{
					"env": &str,
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
					"version": int64(1),
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
			"map with template key": {
				in: map[string]string{
					`{{"1"}}`: "one",
				},
				expected: map[string]string{
					"1": "one",
				},
			},
			"[]string": {
				in:       []string{`{{"one"}}`, "two", `{{"three"}}`},
				expected: []string{"one", "two", "three"},
			},
			"[]*string": {
				in:       []*string{&tmpl},
				expected: []*string{&str},
			},
			"[]interface{}": {
				in:       []interface{}{`{{"one"}}`, `{{1}}`, nil},
				expected: []interface{}{"one", int64(1), nil},
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
					yaml.MapItem{
						Key:   "{{1}}",
						Value: "one",
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
					yaml.MapItem{
						Key:   int64(1),
						Value: "one",
					},
				},
			},
			"yaml.MapSlice (Key is nil)": {
				in: yaml.MapSlice{
					yaml.MapItem{
						Key:   nil,
						Value: "value",
					},
				},
				expected: yaml.MapSlice{
					yaml.MapItem{
						Key:   nil,
						Value: "value",
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
			"zero struct": {
				in:       s{},
				expected: s{},
			},
			"struct": {
				in: s{
					Str:        tmpl,
					StrPtr:     &tmpl,
					unexported: tmpl,
				},
				expected: s{
					Str:        str,
					StrPtr:     &str,
					unexported: tmpl, // can't set a value to unexported fileld
				},
			},
			"struct pointer": {
				in: &s{
					Str:        tmpl,
					StrPtr:     &tmpl,
					unexported: tmpl,
				},
				expected: &s{
					Str:        str,
					StrPtr:     &str,
					unexported: tmpl, // can't set a value to unexported fileld
				},
			},
			"pointer to interface{}": {
				in:       &iface,
				expected: "test",
			},
			"variable is a template string": {
				in:       "{{a}}",
				expected: "test",
				vars: map[string]string{
					"a": "{{b}}",
					"b": "{{c}}",
					"c": "test",
				},
			},
			"left arrow function (map)": {
				in: map[string]interface{}{
					"{{echo <-}}": map[string]interface{}{
						"message": map[string]interface{}{
							"{{join <-}}": map[string]interface{}{
								"prefix": "pre-",
								"text": map[string]interface{}{
									"{{call <-}}": map[string]interface{}{
										"f":   "{{f}}",
										"arg": "{{text}}",
									},
								},
								"suffix": "-suf",
							},
						},
					},
				},
				expected: "pre-test-suf",
				vars: map[string]interface{}{
					"echo": &echoFunc{},
					"join": &joinFunc{},
					"call": &callFunc{},
					"f":    func(s string) string { return s },
					"text": "test",
				},
			},
			"left arrow function (yaml.MapSlice)": {
				in: yaml.MapSlice{
					yaml.MapItem{
						Key: "{{echo <-}}",
						Value: yaml.MapSlice{
							yaml.MapItem{
								Key: "message",
								Value: yaml.MapSlice{
									yaml.MapItem{
										Key: "{{join <-}}",
										Value: yaml.MapSlice{
											yaml.MapItem{
												Key:   "prefix",
												Value: "pre-",
											},
											yaml.MapItem{
												Key: "text",
												Value: yaml.MapSlice{
													yaml.MapItem{
														Key: "{{call <-}}",
														Value: yaml.MapSlice{
															yaml.MapItem{
																Key:   "f",
																Value: "{{f}}",
															},
															yaml.MapItem{
																Key:   "arg",
																Value: "{{text}}",
															},
														},
													},
												},
											},
											yaml.MapItem{
												Key:   "suffix",
												Value: "-suf",
											},
										},
									},
								},
							},
						},
					},
				},
				expected: "pre-test-suf",
				vars: map[string]interface{}{
					"echo": &echoFunc{},
					"join": &joinFunc{},
					"call": &callFunc{},
					"f":    func(s string) string { return s },
					"text": "test",
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				got, err := Execute(ctx, test.in, test.vars)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.expected, got, cmp.AllowUnexported(s{})); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			in            any
			expectedError string
			vars          any
		}{
			"map: invalid key": {
				in: map[any]any{
					`{{foo}}`: "bar",
				},
				expectedError: `failed to execute: {{foo}}: ".foo" not found`,
			},
			"map: invalid value": {
				in: map[any]any{
					"foo": `{{bar}}`,
				},
				expectedError: `.'foo': failed to execute: {{bar}}: ".bar" not found`,
			},
			"map: invalid left arrow function": {
				in: map[any]any{
					`{{"foo"}}`: map[any]any{
						"{{bar <-}}": "{{baz}}",
					},
				},
				expectedError: `.'{{"foo"}}': failed to execute: {{bar <-}}: ".bar" not found`,
			},
			"map: too many left arrow functions": {
				in: map[any]any{
					`{{"foo"}}`: map[any]any{
						"{{bar <-}}": "",
						"{{baz <-}}": "",
					},
				},
				expectedError: `.'{{"foo"}}': invalid left arrow function call`,
				vars: map[string]any{
					"bar": &callFunc{},
					"baz": &callFunc{},
				},
			},
			"map: invalid left arrow function argument": {
				in: map[any]any{
					`{{"foo"}}`: map[any]any{
						"{{bar <-}}": "{{baz}}",
					},
				},
				expectedError: `.'{{"foo"}}'.'{{bar <-}}': failed to execute left arrow function: failed to execute: {{baz}}: ".baz" not found`,
				vars: map[string]any{
					"bar": &callFunc{},
				},
			},
			"yaml.MapSlice: invalid key": {
				in: yaml.MapSlice{
					yaml.MapItem{
						Key:   `{{foo}}`,
						Value: "bar",
					},
				},
				expectedError: `failed to execute: {{foo}}: ".foo" not found`,
			},
			"yaml.MapSlice: invalid value": {
				in: yaml.MapSlice{
					yaml.MapItem{
						Key:   "foo",
						Value: `{{bar}}`,
					},
				},
				expectedError: `.'foo': failed to execute: {{bar}}: ".bar" not found`,
			},
			"yaml.MapSlice: invalid left arrow function": {
				in: yaml.MapSlice{
					yaml.MapItem{
						Key: `{{"foo"}}`,
						Value: yaml.MapSlice{
							yaml.MapItem{
								Key:   "{{bar <-}}",
								Value: "{{baz}}",
							},
						},
					},
				},
				expectedError: `.'{{"foo"}}': failed to execute: {{bar <-}}: ".bar" not found`,
			},
			"yaml.MapSlice: too many left arrow functions": {
				in: yaml.MapSlice{
					yaml.MapItem{
						Key: `{{"foo"}}`,
						Value: yaml.MapSlice{
							yaml.MapItem{
								Key:   "{{bar <-}}",
								Value: "",
							},
							yaml.MapItem{
								Key:   "{{baz <-}}",
								Value: "",
							},
						},
					},
				},
				expectedError: `.'{{"foo"}}': invalid left arrow function call`,
				vars: map[string]any{
					"bar": &callFunc{},
					"baz": &callFunc{},
				},
			},
			"yaml.MapSlice: invalid left arrow function argument": {
				in: yaml.MapSlice{
					yaml.MapItem{
						Key: `{{"foo"}}`,
						Value: yaml.MapSlice{
							yaml.MapItem{
								Key:   "{{bar <-}}",
								Value: "{{baz}}",
							},
						},
					},
				},
				expectedError: `.'{{"foo"}}'.'{{bar <-}}': failed to execute left arrow function: failed to execute: {{baz}}: ".baz" not found`,
				vars: map[string]any{
					"bar": &callFunc{},
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, err := Execute(ctx, test.in, test.vars)
				if err == nil {
					t.Fatal("no error")
				}
				if got, expect := err.Error(), test.expectedError; got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
			})
		}
	})
}

func TestReplaceFuncs(t *testing.T) {
	v := yaml.MapSlice{
		yaml.MapItem{
			Key:   "foo",
			Value: func() {},
		},
		yaml.MapItem{
			Key: "bar",
			Value: map[string]any{
				"hoge": func() {},
			},
		},
		yaml.MapItem{
			Key: "baz",
			Value: []any{
				&callArg{
					F:   func() {},
					Arg: "test",
				},
			},
		},
	}
	if _, err := yaml.Marshal(v); err == nil {
		t.Fatal("no error")
	}

	s := &funcStash{}
	vv, err := replaceFuncs(reflect.ValueOf(v), s)
	if err != nil {
		t.Fatalf("failed to replace funcs: %s", err)
	}
	if b, err := yaml.Marshal(vv.Interface()); err != nil {
		t.Fatalf("failed to marshal: %s", err)
	} else if got, expect := string(b), strings.TrimLeft(`
foo: "{{func-0}}"
bar:
  hoge: "{{func-1}}"
baz:
- f: "{{func-2}}"
  arg: test
`, "\n"); got != expect {
		t.Fatalf("expect %q but got %q", expect, got)
	}

	// don't break the original value
	if _, err := yaml.Marshal(v); err == nil {
		t.Fatal("no error")
	}
}

func TestConvert(t *testing.T) {
	convertToStr := convert(reflect.TypeOf(""))
	t.Run("convert to string", func(t *testing.T) {
		s := "test"
		v, err := convertToStr(reflect.ValueOf(&s), nil)
		if err != nil {
			t.Fatalf("failed to convert: %s", err)
		}
		if got, expect := v.Type().Kind(), reflect.String; got != expect {
			t.Fatalf("expect %s but got %s", expect, got)
		}
	})
	t.Run("error", func(t *testing.T) {
		_, err := convertToStr(reflect.Value{}, errors.New("execute() failed"))
		if err == nil {
			t.Fatal("no error")
		}
	})
}
