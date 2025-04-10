package reflectutil

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestConvertStringsMap(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			v      any
			expect map[string][]string
		}{
			"map[string]string": {
				v:      map[string]string{"A": "a"},
				expect: map[string][]string{"A": {"a"}},
			},
			"map[string][]string": {
				v:      map[string][]string{"A": {"a"}},
				expect: map[string][]string{"A": {"a"}},
			},
			"map[int]int": {
				v:      map[int]int{0: 0},
				expect: map[string][]string{"0": {"0"}},
			},
			"map[bool]bool": {
				v:      map[bool]bool{true: true},
				expect: map[string][]string{"true": {"true"}},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := ConvertStringsMap(reflect.ValueOf(test.v))
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.expect, got); diff != "" {
					t.Fatalf("differs: (-want +got)\n%s", diff)
				}
			})
		}
	})
	t.Run("error", func(t *testing.T) {
		tests := map[string]struct {
			v any
		}{
			"nil": {
				v: nil,
			},
			"int": {
				v: 555,
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				if _, err := ConvertStringsMap(reflect.ValueOf(test.v)); err == nil {
					t.Fatal("expected error but no error")
				}
			})
		}
	})
}

func TestConvertStrings(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			v      any
			expect []string
		}{
			"string": {
				v:      "test",
				expect: []string{"test"},
			},
			"[]string": {
				v:      []string{"1", "2"},
				expect: []string{"1", "2"},
			},
			"int": {
				v:      1,
				expect: []string{"1"},
			},
			"[]interface": {
				v: []any{
					true, false,
					1, int8(2), int16(3), int32(4), int64(5),
					uint(6), uint8(7), uint16(8), uint32(9), uint64(10),
				},
				expect: []string{
					"true", "false",
					"1", "2", "3", "4", "5",
					"6", "7", "8", "9", "10",
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := ConvertStrings(reflect.ValueOf(test.v))
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.expect, got); diff != "" {
					t.Fatalf("differs: (-want +got)\n%s", diff)
				}
			})
		}
	})
	t.Run("error", func(t *testing.T) {
		tests := map[string]struct {
			v any
		}{
			"nil": {
				v: nil,
			},
			"nil (string pointer)": {
				v: (*string)(nil),
			},
			"float64": {
				v: 1.2,
			},
			"[]interface{}": {
				v: []any{nil},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				if _, err := ConvertStrings(reflect.ValueOf(test.v)); err == nil {
					t.Fatal("expected error but no error")
				}
			})
		}
	})
}

func TestConvertString(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			v      any
			expect string
		}{
			"string": {
				v:      "test",
				expect: "test",
			},
			"bool": {
				v:      true,
				expect: "true",
			},
			"integer": {
				v:      1,
				expect: "1",
			},
			"[]byte": {
				v:      []byte("test"),
				expect: "test",
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := ConvertString(reflect.ValueOf(test.v))
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if got != test.expect {
					t.Errorf("expect %q but got %q", test.expect, got)
				}
			})
		}
	})
	t.Run("error", func(t *testing.T) {
		tests := map[string]struct {
			v any
		}{
			"nil": {
				v: nil,
			},
			"nil (string pointer)": {
				v: (*string)(nil),
			},
			"float64": {
				v: 1.2,
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				if _, err := ConvertString(reflect.ValueOf(test.v)); err == nil {
					t.Fatal("expected error but no error")
				}
			})
		}
	})
}
