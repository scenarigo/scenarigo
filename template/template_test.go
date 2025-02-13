package template

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/scenarigo/scenarigo/internal/testutil"
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
	tests := map[string]executeTestCase{
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
		"implicit concatenation": {
			str:    `{{1}}2{{3}}`,
			expect: "123",
		},
		"string": {
			str:    `{{"foo"}}`,
			expect: "foo",
		},
		"integer": {
			str:    "{{1}}",
			expect: int64(1),
		},
		"float": {
			str:    "{{1.23}}",
			expect: 1.23,
		},
		"true": {
			str:    "{{true}}",
			expect: true,
		},
		"false": {
			str:    "{{false}}",
			expect: false,
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
				"f": func(s string) string { return s },
			},
			expect: "ok",
		},
		"call function that have argument required cast": {
			str: `{{f(1, 2, 3, 4, 5)}}`,
			data: map[string]func(int, int8, int16, int32, int64) int{
				"f": func(a0 int, a1 int8, a2 int16, a3 int32, a4 int64) int {
					return a0 + int(a1) + int(a2) + int(a3) + int(a4)
				},
			},
			expect: 15,
		},
		"call function that have variadic arguments": {
			str: `{{f(1, 2, 3, 4, 5)}}`,
			data: map[string]func(int, ...float32) int{
				"f": func(a0 int, args ...float32) int {
					sum := a0
					for _, a := range args {
						sum += int(a)
					}
					return sum
				},
			},
			expect: 15,
		},
		"function call (with nil error)": {
			str: `{{f("ok")}}`,
			data: map[string]interface{}{
				"f": func(s string) (string, error) { return s, nil },
			},
			expect: "ok",
		},
		"invalid function argument": {
			str: `{{f(1, 2, 3)}}`,
			data: map[string]func(int, int) int{
				"f": func(a0, a1 int) int {
					return a0 + a1
				},
			},
			expectError: "expected function argument number is 2 but specified 3 arguments",
		},
		"invalid function argument ( variadic arguments )": {
			str: `{{f()}}`,
			data: map[string]func(int, ...float32) int{
				"f": func(a0 int, args ...float32) int {
					sum := a0
					for _, a := range args {
						sum += int(a)
					}
					return sum
				},
			},
			expectError: "too few arguments to function: expected minimum argument number is 1. but specified 0 arguments",
		},
		"invalid function argument type (ident)": {
			str: `{{fn("1")}}`,
			data: map[string]func(int){
				"fn": func(a int) {},
			},
			expectError: "can't use string as int in arguments[0] to fn",
		},
		"invalid function argument type (selector)": {
			str: `{{m.fn("1")}}`,
			data: map[string]interface{}{
				"m": map[string]func(int){
					"fn": func(a int) {},
				},
			},
			expectError: "can't use string as int in arguments[0] to fn",
		},
		"function call (second value is not an error)": {
			str: `{{f()}}`,
			data: map[string]interface{}{
				"f": func() (interface{}, interface{}) { return nil, error(nil) },
			},
			expectError: "second returned value must be an error",
		},
		"function call (with error)": {
			str: `{{f()}}`,
			data: map[string]interface{}{
				"f": func() (interface{}, error) { return nil, errors.New("f() error") },
			},
			expectError: "f() error",
		},

		"method call": {
			str: `{{s.Echo("a") + s.Repeat("b") + p.Echo("c") + p.Self().Repeat(d)}}`,
			data: map[string]interface{}{
				"s": echoStruct{},
				"p": &echoStruct{},
				"d": testutil.ToPtr("d"),
			},
			expect: "abbcdd",
		},
		"method not found": {
			str: `{{p.Invalid()}}`,
			data: map[string]interface{}{
				"p": &echoStruct{},
			},
			expectError: `failed to execute: {{p.Invalid()}}: ".Invalid" not found`,
		},
		"invalid method argument": {
			str: `{{p.Repeat(a)}}`,
			data: map[string]interface{}{
				"p": &echoStruct{},
				"a": 1.2,
			},
			expectError: "can't use float64 as string in arguments[0] to Repeat",
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
		"left arrow func with function in argument": {
			str: strings.Trim(`
{{exec <-}}: '{{f}}'
`, "\n"),
			data: map[string]interface{}{
				"exec": &execFunc{},
				"f":    func() string { return "hello" },
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
				"f":    func(s string) string { return s },
				"text": "test",
			},
			expect: "preout-prein-test-sufin-sufout",
		},
		"left arrow func with the non-string argument": {
			str: strings.Trim(`
{{join <-}}: '{{arg}}'
`, "\n"),
			data: map[string]interface{}{
				"join": &joinFunc{},
				"arg": map[string]interface{}{
					"prefix": "pre-",
					"text":   "{{text}}",
					"suffix": "-suf",
				},
				"text": 0,
			},
			expect: "pre-0-suf",
		},
		"left arrow func (complex)": {
			str: strings.Trim(`
{{echo <-}}:
  message: |-
    {{join <-}}:
      prefix: pre-
      text: |-
        {{call <-}}:
          f: '{{f}}'
          arg: '{{text}}'
      suffix: -suf
`, "\n"),
			data: map[string]interface{}{
				"echo": &echoFunc{},
				"join": &joinFunc{},
				"call": &callFunc{},
				"f":    func(s string) string { return s },
				"text": "test",
			},
			expect: "pre-test-suf",
		},
		"callable": {
			str: "{{f(a, b)}}",
			data: map[string]interface{}{
				"f": &joinFunc{},
				"a": "foo",
				"b": "bar",
			},
			expect: "foobar",
		},
		"size(string)": {
			str:    `{{size("test")}}`,
			expect: int64(4),
		},
		"size([]int)": {
			str: `{{size(v)}}`,
			data: map[string]interface{}{
				"v": []int{0},
			},
			expect: int64(1),
		},
		"size(struct)": {
			str: `{{size(v)}}`,
			data: map[string]interface{}{
				"v": struct{}{},
			},
			expectError: "failed to execute: {{size(v)}}: size(any[struct {}]) is not defined",
		},
		"size(nil)": {
			str: `{{size(v)}}`,
			data: map[string]any{
				"v": nil,
			},
			expectError: "failed to execute: {{size(v)}}: size(nil) is not defined",
		},
		"not found": {
			str:         "{{a.b[1]}}",
			expectError: `".a.b[1]" not found`,
		},
		"panic": {
			str: "{{panic()}}",
			data: map[string]interface{}{
				"panic": func() { panic("omg") },
			},
			expectError: "omg",
		},
	}
	runExecute(t, tests)
}

func TestTemplate_Execute_UnaryExpr(t *testing.T) {
	tests := map[string]executeTestCase{
		"!true": {
			str:    "{{!true}}",
			expect: false,
		},
		"!1": {
			str:         "{{!1}}",
			expectError: `failed to execute: {{!1}}: invalid operation: operator ! not defined on int(1)`,
		},

		"negative int": {
			str: "{{-v}}",
			data: map[string]interface{}{
				"v": math.MaxInt,
			},
			expect: int64(-math.MaxInt),
		},
		"negative float": {
			str: "{{-v}}",
			data: map[string]interface{}{
				"v": float64(math.MaxFloat64),
			},
			expect: -float64(math.MaxFloat64),
		},
		"negative duration": {
			str: "{{-v}}",
			data: map[string]interface{}{
				"v": time.Second,
			},
			expect: -time.Second,
		},
		"negative uint": {
			str: "{{-v}}",
			data: map[string]interface{}{
				"v": uint(math.MaxInt),
			},
			expectError: `failed to execute: {{-v}}: invalid operation: operator - not defined on uint(9223372036854775807)`,
		},

		"defined": {
			str: "{{defined(a.b)}}",
			data: map[string]any{
				"a": map[string]any{
					"b": `{{test}}`,
				},
			},
			expect: true,
		},
		"not defined": {
			str:    "{{defined(a.b)}}",
			expect: false,
		},
		"invalid argument to defined()": {
			str:         "{{defined(true)}}",
			expectError: "failed to execute: {{defined(true)}}: invalid argument to defined()",
		},
	}
	runExecute(t, tests)
}

func TestTemplate_Execute_BinaryExpr(t *testing.T) {
	t.Run("+", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"add ints": {
				str: `{{v + int(1)}}`,
				data: map[string]interface{}{
					"v": int64(math.MaxInt64 - 1),
				},
				expect: int64(math.MaxInt64),
			},
			"add uints": {
				str:    `{{uint(1) + uint(2)}}`,
				expect: uint64(3),
			},
			"add floats": {
				str:    `{{1.0 + 0.23}}`,
				expect: 1.23,
			},
			"add strings": {
				str:    `foo-{{ "bar" + "-" + "baz" }}`,
				expect: "foo-bar-baz",
			},
			"add bytes": {
				str: "{{a + b}}",
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: []byte("ab"),
			},
			"time + duration": {
				str:    `{{time("2009-11-10T23:00:00Z") + duration("1s")}}`,
				expect: time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
			},
			"add duration": {
				str:    `{{duration("2m") + duration("1h3s")}}`,
				expect: time.Hour + 2*time.Minute + 3*time.Second,
			},
			"failed to add bools": {
				str:         `{{true + false}}`,
				expectError: "failed to execute: {{true + false}}: invalid operation: bool(true) + bool(false) not defined",
			},
		}
		runExecute(t, tests)
	})

	t.Run("-", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"sub positive int": {
				str: `{{v - int(1)}}`,
				data: map[string]interface{}{
					"v": int64(math.MinInt64 + 1),
				},
				expect: int64(math.MinInt64),
			},
			"sub uints": {
				str:    `{{uint(2) - uint(1)}}`,
				expect: uint64(1),
			},
			"sub floats": {
				str:    `{{1.0 - 0.23}}`,
				expect: 0.77,
			},
			"time - duration": {
				str:    `{{time("2009-11-10T23:00:00Z") - duration("1s")}}`,
				expect: time.Date(2009, time.November, 10, 22, 59, 59, 0, time.UTC),
			},
			"sub durations": {
				str: `{{x - y}}`,
				data: map[string]interface{}{
					"x": time.Second,
					"y": time.Minute,
				},
				expect: -59 * time.Second,
			},
			"failed to sub bools": {
				str:         `{{true - false}}`,
				expectError: "failed to execute: {{true - false}}: invalid operation: bool(true) - bool(false) not defined",
			},
		}
		runExecute(t, tests)
	})

	t.Run("*", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"mul ints": {
				str:    `{{2 * 3}}`,
				expect: int64(6),
			},
			"mul uints": {
				str:    `{{uint(2) * uint(3)}}`,
				expect: uint64(6),
			},
			"mul floats": {
				str:    `{{1.2 * 3.4}}`,
				expect: float64(4.08),
			},
			"failed to mul bools": {
				str:         `{{true * false}}`,
				expectError: "failed to execute: {{true * false}}: invalid operation: bool(true) * bool(false) not defined",
			},
		}
		runExecute(t, tests)
	})

	t.Run("/", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"quo ints": {
				str:    `{{3 / -2}}`,
				expect: int64(-1),
			},
			"quo uints": {
				str:    `{{uint(3) / uint(2)}}`,
				expect: uint64(1),
			},
			"quo floats": {
				str:    `{{float(3) / float(2)}}`,
				expect: float64(1.5),
			},
			"failed to quo bools": {
				str:         `{{true / false}}`,
				expectError: "failed to execute: {{true / false}}: invalid operation: bool(true) / bool(false) not defined",
			},
		}
		runExecute(t, tests)
	})

	t.Run("%", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"rem ints": {
				str:    `{{3 % -2}}`,
				expect: int64(1),
			},
			"rem uints": {
				str:    `{{uint(3) % uint(2)}}`,
				expect: uint64(1),
			},
			"failed to rem bools": {
				str:         `{{true % false}}`,
				expectError: "failed to execute: {{true % false}}: invalid operation: bool(true) % bool(false) not defined",
			},
		}
		runExecute(t, tests)
	})

	t.Run("==", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"true==true": {
				str:    `{{true==true}}`,
				expect: true,
			},
			"true==false": {
				str:    `{{true==false}}`,
				expect: false,
			},
			`1==1`: {
				str:    `{{1==1}}`,
				expect: true,
			},
			`1==2`: {
				str:    `{{1==2}}`,
				expect: false,
			},
			`1.1==1.1`: {
				str:    `{{1.1==1.1}}`,
				expect: true,
			},
			`1.1==2.2`: {
				str:    `{{1.1==2.2}}`,
				expect: false,
			},
			`"a"=="a"`: {
				str:    `{{"a"=="a"}}`,
				expect: true,
			},
			`"a"=="b"`: {
				str:    `{{"a"=="b"}}`,
				expect: false,
			},
			`bytes("a")==bytes("a")`: {
				str: `{{a==a}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
				},
				expect: true,
			},
			`bytes("a")==bytes("b")`: {
				str: `{{a==b}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: false,
			},
			`time == time (true)`: {
				str: `{{v==v}}`,
				data: map[string]interface{}{
					"v": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
				},
				expect: true,
			},
			`time == time (false)`: {
				str: `{{x==y}}`,
				data: map[string]interface{}{
					"x": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					"y": time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
				},
				expect: false,
			},
			`duration("1s")==duration("1s")`: {
				str: `{{v==v}}`,
				data: map[string]interface{}{
					"v": time.Second,
				},
				expect: true,
			},
			`duration("1s")==duration("1ms")`: {
				str: `{{x==y}}`,
				data: map[string]interface{}{
					"x": time.Second,
					"y": time.Millisecond,
				},
				expect: false,
			},
			"nil==nil": {
				str: `{{v==v}}`,
				data: map[string]interface{}{
					"v": nil,
				},
				expect: true,
			},
		}
		runExecute(t, tests)
	})

	t.Run("!=", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"true!=true": {
				str:    `{{true!=true}}`,
				expect: false,
			},
			"true!=false": {
				str:    `{{true!=false}}`,
				expect: true,
			},
			`1!=1`: {
				str:    `{{1!=1}}`,
				expect: false,
			},
			`1!=2`: {
				str:    `{{1!=2}}`,
				expect: true,
			},
			`1.1!=1.1`: {
				str:    `{{1.1!=1.1}}`,
				expect: false,
			},
			`1.1!=2.2`: {
				str:    `{{1.1!=2.2}}`,
				expect: true,
			},
			`"a"!="a"`: {
				str:    `{{"a"!="a"}}`,
				expect: false,
			},
			`"a"!="b"`: {
				str:    `{{"a"!="b"}}`,
				expect: true,
			},
			`bytes("a")!=bytes("a")`: {
				str: `{{a!=a}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
				},
				expect: false,
			},
			`bytes("a")!=bytes("b")`: {
				str: `{{a!=b}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: true,
			},
			`time != time (false)`: {
				str: `{{v!=v}}`,
				data: map[string]interface{}{
					"v": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
				},
				expect: false,
			},
			`time != time (true)`: {
				str: `{{x!=y}}`,
				data: map[string]interface{}{
					"x": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					"y": time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
				},
				expect: true,
			},
			`duration("1s")!=duration("1s")`: {
				str: `{{v!=v}}`,
				data: map[string]interface{}{
					"v": time.Second,
				},
				expect: false,
			},
			`duration("1s")!=duration("1ms")`: {
				str: `{{x!=y}}`,
				data: map[string]interface{}{
					"x": time.Second,
					"y": time.Millisecond,
				},
				expect: true,
			},
			"nil!=nil": {
				str: `{{v!=v}}`,
				data: map[string]interface{}{
					"v": nil,
				},
				expect: false,
			},
		}
		runExecute(t, tests)
	})

	t.Run("<", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"1<2": {
				str:    `{{1<2}}`,
				expect: true,
			},
			"1<1": {
				str:    `{{1<1}}`,
				expect: false,
			},
			"2<1": {
				str:    `{{2<1}}`,
				expect: false,
			},
			"uint(1)<uint(2)": {
				str:    `{{uint(1)<uint(2)}}`,
				expect: true,
			},
			"uint(1)<uint(1)": {
				str:    `{{uint(1)<uint(1)}}`,
				expect: false,
			},
			"uint(2)<uint(1)": {
				str:    `{{uint(2)<uint(1)}}`,
				expect: false,
			},
			"1.1<2.2": {
				str:    `{{1.1<2.2}}`,
				expect: true,
			},
			"1.1<1.1": {
				str:    `{{1.1<1.1}}`,
				expect: false,
			},
			"2.2<1.1": {
				str:    `{{2.2<1.1}}`,
				expect: false,
			},
			`"a"<"b"`: {
				str:    `{{"a"<"b"}}`,
				expect: true,
			},
			`"a"<"a"`: {
				str:    `{{"a"<"a"}}`,
				expect: false,
			},
			`"b"<"a"`: {
				str:    `{{"b"<"a"}}`,
				expect: false,
			},
			`bytes("a")<bytes("b")`: {
				str: `{{a<b}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: true,
			},
			`bytes("a")<bytes("a")`: {
				str: `{{a<a}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
				},
				expect: false,
			},
			`bytes("b")<bytes("a")`: {
				str: `{{b<a}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: false,
			},
			`time < time (true)`: {
				str: `{{x<y}}`,
				data: map[string]interface{}{
					"x": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					"y": time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
				},
				expect: true,
			},
			`time < time (false)`: {
				str: `{{y<x}}`,
				data: map[string]interface{}{
					"x": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					"y": time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
				},
				expect: false,
			},
			`duration("1ms")<duration("1s")`: {
				str: `{{x<y}}`,
				data: map[string]interface{}{
					"x": time.Millisecond,
					"y": time.Second,
				},
				expect: true,
			},
			`duration("1s")<duration("1s")`: {
				str: `{{v<v}}`,
				data: map[string]interface{}{
					"v": time.Second,
				},
				expect: false,
			},
		}
		runExecute(t, tests)
	})

	t.Run("<=", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"1<=2": {
				str:    `{{1<=2}}`,
				expect: true,
			},
			"1<=1": {
				str:    `{{1<=1}}`,
				expect: true,
			},
			"2<=1": {
				str:    `{{2<=1}}`,
				expect: false,
			},
			"uint(1)<=uint(2)": {
				str:    `{{uint(1)<=uint(2)}}`,
				expect: true,
			},
			"uint(1)<=uint(1)": {
				str:    `{{uint(1)<=uint(1)}}`,
				expect: true,
			},
			"uint(2)<=uint(1)": {
				str:    `{{uint(2)<=uint(1)}}`,
				expect: false,
			},
			"1.1<=2.2": {
				str:    `{{1.1<=2.2}}`,
				expect: true,
			},
			"1.1<=1.1": {
				str:    `{{1.1<=1.1}}`,
				expect: true,
			},
			"2.2<=1.1": {
				str:    `{{2.2<=1.1}}`,
				expect: false,
			},
			`"a"<="b"`: {
				str:    `{{"a"<="b"}}`,
				expect: true,
			},
			`"a"<="a"`: {
				str:    `{{"a"<="a"}}`,
				expect: true,
			},
			`"b"<="a"`: {
				str:    `{{"b"<="a"}}`,
				expect: false,
			},
			`bytes("a")<=bytes("b")`: {
				str: `{{a<=b}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: true,
			},
			`bytes("a")<=bytes("a")`: {
				str: `{{a<=a}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
				},
				expect: true,
			},
			`bytes("b")<=bytes("a")`: {
				str: `{{b<=a}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: false,
			},
			`time <= time (true)`: {
				str: `{{x<=y}}`,
				data: map[string]interface{}{
					"x": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					"y": time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
				},
				expect: true,
			},
			`time < time (false)`: {
				str: `{{y<=x}}`,
				data: map[string]interface{}{
					"x": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					"y": time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
				},
				expect: false,
			},
			`duration("1s")<=duration("1s")`: {
				str: `{{v<=v}}`,
				data: map[string]interface{}{
					"v": time.Second,
				},
				expect: true,
			},
			`duration("1s")<=duration("1ms")`: {
				str: `{{x<=y}}`,
				data: map[string]interface{}{
					"x": time.Second,
					"y": time.Millisecond,
				},
				expect: false,
			},
		}
		runExecute(t, tests)
	})

	t.Run(">", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"1>2": {
				str:    `{{1>2}}`,
				expect: false,
			},
			"1>1": {
				str:    `{{1>1}}`,
				expect: false,
			},
			"2>1": {
				str:    `{{2>1}}`,
				expect: true,
			},
			"uint(1)>uint(2)": {
				str:    `{{uint(1)>uint(2)}}`,
				expect: false,
			},
			"uint(1)>uint(1)": {
				str:    `{{uint(1)>uint(1)}}`,
				expect: false,
			},
			"uint(2)>uint(1)": {
				str:    `{{uint(2)>uint(1)}}`,
				expect: true,
			},
			"1.1>2.2": {
				str:    `{{1.1>2.2}}`,
				expect: false,
			},
			"1.1>1.1": {
				str:    `{{1.1>1.1}}`,
				expect: false,
			},
			"2.2>1.1": {
				str:    `{{2.2>1.1}}`,
				expect: true,
			},
			`"a">"b"`: {
				str:    `{{"a">"b"}}`,
				expect: false,
			},
			`"a">"a"`: {
				str:    `{{"a">"a"}}`,
				expect: false,
			},
			`"b">"a"`: {
				str:    `{{"b">"a"}}`,
				expect: true,
			},
			`bytes("a")>bytes("b")`: {
				str: `{{a>b}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: false,
			},
			`bytes("a")>bytes("a")`: {
				str: `{{a>a}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
				},
				expect: false,
			},
			`bytes("b")>bytes("a")`: {
				str: `{{b>a}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: true,
			},
			`time > time (false)`: {
				str: `{{x>y}}`,
				data: map[string]interface{}{
					"x": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					"y": time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
				},
				expect: false,
			},
			`time > time (true)`: {
				str: `{{y>x}}`,
				data: map[string]interface{}{
					"x": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					"y": time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
				},
				expect: true,
			},
			`duration("1s")>duration("1ms")`: {
				str: `{{x>y}}`,
				data: map[string]interface{}{
					"x": time.Second,
					"y": time.Millisecond,
				},
				expect: true,
			},
			`duration("1s")>duration("1s")`: {
				str: `{{v>v}}`,
				data: map[string]interface{}{
					"v": time.Second,
				},
				expect: false,
			},
		}
		runExecute(t, tests)
	})

	t.Run(">=", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"1>=2": {
				str:    `{{1>=2}}`,
				expect: false,
			},
			"1>=1": {
				str:    `{{1>=1}}`,
				expect: true,
			},
			"2>=1": {
				str:    `{{2>=1}}`,
				expect: true,
			},
			"uint(1)>=uint(2)": {
				str:    `{{uint(1)>=uint(2)}}`,
				expect: false,
			},
			"uint(1)>=uint(1)": {
				str:    `{{uint(1)>=uint(1)}}`,
				expect: true,
			},
			"uint(2)>=uint(1)": {
				str:    `{{uint(2)>=uint(1)}}`,
				expect: true,
			},
			"1.1>=2.2": {
				str:    `{{1.1>=2.2}}`,
				expect: false,
			},
			"1.1>=1.1": {
				str:    `{{1.1>=1.1}}`,
				expect: true,
			},
			"2.2>=1.1": {
				str:    `{{2.2>=1.1}}`,
				expect: true,
			},
			`"a">="b"`: {
				str:    `{{"a">="b"}}`,
				expect: false,
			},
			`"a">="a"`: {
				str:    `{{"a">="a"}}`,
				expect: true,
			},
			`"b">="a"`: {
				str:    `{{"b">="a"}}`,
				expect: true,
			},
			`bytes("a")>=bytes("b")`: {
				str: `{{a>=b}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: false,
			},
			`bytes("a")>=bytes("a")`: {
				str: `{{a>=a}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
				},
				expect: true,
			},
			`bytes("b")>=bytes("a")`: {
				str: `{{b>=a}}`,
				data: map[string]interface{}{
					"a": []byte("a"),
					"b": []byte("b"),
				},
				expect: true,
			},
			`time >= time (false)`: {
				str: `{{x>=y}}`,
				data: map[string]interface{}{
					"x": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					"y": time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
				},
				expect: false,
			},
			`time >= time (true)`: {
				str: `{{y>=x}}`,
				data: map[string]interface{}{
					"x": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					"y": time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC),
				},
				expect: true,
			},
			`duration("1s")>=duration("1s")`: {
				str: `{{v>=v}}`,
				data: map[string]interface{}{
					"v": time.Second,
				},
				expect: true,
			},
			`duration("1ms")>=duration("1s")`: {
				str: `{{x>=y}}`,
				data: map[string]interface{}{
					"x": time.Millisecond,
					"y": time.Second,
				},
				expect: false,
			},
		}
		runExecute(t, tests)
	})

	t.Run("&&", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"true&&true": {
				str:    `{{true&&true}}`,
				expect: true,
			},
			"true&&false": {
				str:    `{{true&&false}}`,
				expect: false,
			},
		}
		runExecute(t, tests)
	})

	t.Run("||", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"true||false": {
				str:    `{{true||false}}`,
				expect: true,
			},
			"false||false": {
				str:    `{{false||false}}`,
				expect: false,
			},
		}
		runExecute(t, tests)
	})

	t.Run("??", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"lhs defined": {
				str: `{{a.b ?? true + "default"}}`, // the right-hand side is not executed
				data: map[string]interface{}{
					"a": map[string]interface{}{
						"b": "something",
					},
				},
				expect: "something",
			},
			"lhs nil": {
				str: `{{a.b ?? "default"}}`,
				data: map[string]interface{}{
					"a": map[string]interface{}{
						"b": nil,
					},
				},
				expect: "default",
			},
			"lhs typed nil": {
				str: `{{a.b ?? "default"}}`,
				data: map[string]interface{}{
					"a": map[string]interface{}{
						"b": []int(nil),
					},
				},
				expect: "default",
			},
			"lhs undefined": {
				str:    `{{a.b ?? "default"}}`,
				expect: "default",
			},
			"lhs expr": {
				str: `{{a.b + " new" ?? true + "default"}}`, // the right-hand side is not executed
				data: map[string]interface{}{
					"a": map[string]interface{}{
						"b": "something",
					},
				},
				expect: "something new",
			},
			"lhs expr nil": {
				str: `{{(true ? a.b : a.b) ?? "default"}}`,
				data: map[string]interface{}{
					"a": map[string]interface{}{
						"b": nil,
					},
				},
				expect: "default",
			},
			"lhs expr typed nil": {
				str: `{{(true ? a.b : a.b) ?? "default"}}`,
				data: map[string]interface{}{
					"a": map[string]interface{}{
						"b": []int(nil),
					},
				},
				expect: "default",
			},
			"lhs expr contains undefined variable": {
				str:         `{{a.b + 1 ?? "default"}}`,
				expectError: "failed to execute: {{a.b + 1 ?? \"default\"}}: invalid operation: invalid operation: \".a.b\" not found",
			},
		}
		runExecute(t, tests)
	})

	t.Run("? :", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"true ? 1 : 2": {
				str:    `{{true ? 1 : 2}}`,
				expect: int64(1),
			},
			"false ? 1 : 2": {
				str:    `{{false ? 1 : 2}}`,
				expect: int64(2),
			},
			"1 ? 1 : 2": {
				str:         `{{1 ? 1 : 2}}`,
				expectError: `failed to execute: {{1 ? 1 : 2}}: invalid operation: operator ? not defined on int(1)`,
			},
			`defined(v) ? v : "default" + true`: {
				// "default" + true should not be evaluated
				str: `{{defined(v) ? v : "default" + true}}`,
				data: map[string]interface{}{
					"v": "override",
				},
				expect: "override",
			},
			`defined(v) ? v + true : "default"`: {
				// v + true should not be evaluated
				str:    `{{defined(v) ? v : "default"}}`,
				expect: "default",
			},
		}
		runExecute(t, tests)
	})

	t.Run("complicated", func(t *testing.T) {
		tests := map[string]executeTestCase{
			"*,/ have precedence over +,-": {
				str:    `{{1 + 2 * 3 / 4 - 5}}`,
				expect: int64(-3),
			},
			"paren expr": {
				str:    `{{(1 + 2) * 3 / (4 - 5)}}`,
				expect: int64(-9),
			},
			"condition with paren": {
				str:    `{{ 1 != 2 && !(false || 1 >= 2)}}`,
				expect: true,
			},
			"conditional operator": {
				str:    `{{1 + 1 <= 2 ? 3 * 3 : 4 / 4}}`,
				expect: int64(9),
			},
		}
		runExecute(t, tests)
	})
}

type executeTestCase struct {
	str         string
	data        interface{}
	expect      interface{}
	expectError string
}

func runExecute(t *testing.T, tests map[string]executeTestCase) {
	t.Helper()
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			tmpl, err := New(test.str)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			i, err := tmpl.Execute(ctx, test.data)
			if test.expectError == "" && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if test.expectError != "" {
				if err == nil {
					t.Fatal("expected error but got no error")
				}
				if got, expected := err.Error(), test.expectError; !strings.Contains(got, expected) {
					t.Errorf("expected error %q but got %q", expected, got)
				}
			}
			if diff := cmp.Diff(test.expect, i); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestLeftArrowFunctionArg(t *testing.T) {
	tests := map[string]struct {
		str    string
		data   map[string]interface{}
		expect interface{}
	}{
		"no template": {
			str: strings.TrimPrefix(`
a: 1
b: 2
`, "\n"),
			expect: map[string]interface{}{
				"a": uint64(1),
				"b": uint64(2),
			},
		},
		"string": {
			str:    `'{{"test"}}'`,
			expect: "test",
		},
		"int": {
			str:    `'{{1}}'`,
			expect: uint64(1),
		},
		"int string": {
			str:    `'{{"1"}}'`,
			expect: "1",
		},
		"map array": {
			str: strings.TrimPrefix(`
users:
  - '{{user}}'
`, "\n"),
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "Alice",
					"age":  20,
				},
			},
			expect: map[string]interface{}{
				"users": []interface{}{
					map[string]interface{}{
						"name": "Alice",
						"age":  uint64(20),
					},
				},
			},
		},
		"map map": {
			str: strings.TrimPrefix(`
admin: '{{user}}'
`, "\n"),
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "Alice",
					"age":  20,
				},
			},
			expect: map[string]interface{}{
				"admin": map[string]interface{}{
					"name": "Alice",
					"age":  uint64(20),
				},
			},
		},
		"complex function call": {
			str: strings.TrimPrefix(`
prefix: pre-
text: |-
  {{call <-}}:
    f: '{{f}}'
    arg: '{{text}}'
suffix: -suf
`, "\n"),
			data: map[string]interface{}{
				"call": &callFunc{},
				"f":    func(s string) string { return s },
				"text": "test",
			},
			expect: map[string]interface{}{
				"prefix": "pre-",
				"text":   "test",
				"suffix": "-suf",
			},
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			lines := []string{"{{dump <-}}:"}
			for _, line := range strings.Split(test.str, "\n") {
				lines = append(lines, fmt.Sprintf("  %s", line))
			}
			tmpl, err := New(strings.Join(lines, "\n"))
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			tmpl.executingLeftArrowExprArg = true
			data := test.data
			if data == nil {
				data = map[string]interface{}{
					"dump": &dumpFunc{},
				}
			} else {
				data["dump"] = &dumpFunc{}
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			v, err := tmpl.Execute(ctx, data)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			s, ok := v.(string)
			if !ok {
				t.Fatalf("expect string but got %T", v)
			}
			var i interface{}
			if err := yaml.Unmarshal([]byte(s), &i); err != nil {
				t.Fatal(err)
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
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			i, err := Execute(ctx, test.i, test.data)
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

type echoStruct struct{}

func (s *echoStruct) Self() *echoStruct {
	return s
}

func (s echoStruct) Echo(str string) interface{} {
	return str
}

func (s *echoStruct) Repeat(str string) string {
	return strings.Repeat(str, 2)
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

var _ Func = &execFunc{}

type execFunc struct{}

func (*execFunc) Exec(in interface{}) (interface{}, error) {
	v := reflect.ValueOf(in)
	if !v.IsValid() {
		return nil, errors.New("invalid value")
	}
	if v.Kind() != reflect.Func {
		return nil, errors.Errorf("arg must be a function: %v", in)
	}
	t := v.Type()
	if n := t.NumIn(); n != 0 {
		return nil, errors.Errorf("number of arguments must be 0 but got %d", n)
	}
	if n := t.NumOut(); n != 1 {
		return nil, errors.Errorf("number of arguments must be 1 but got %d", n)
	}
	return v.Call(nil)[0].Interface(), nil
}

func (*execFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var arg interface{}
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

func (*joinFunc) Call(strs ...string) (string, error) {
	return strings.Join(strs, ""), nil
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

func TestFuncStash(t *testing.T) {
	var s funcStash
	name := s.save("value")
	if s[name] != "value" {
		t.Fatal("failed to save")
	}
}

var _ Func = &dumpFunc{}

type dumpFunc struct{}

func (*dumpFunc) Exec(in interface{}) (interface{}, error) {
	return in, nil
}

func (*dumpFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var arg interface{}
	if err := unmarshal(&arg); err != nil {
		return nil, err
	}
	return arg, nil
}
