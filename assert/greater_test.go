package assert

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

func TestGreater(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Run("number", func(t *testing.T) {
			act := 3
			exp := 2
			for _, actual := range []any{
				act, int8(act), int16(act), int32(act), int64(act),
				uint(act), uint8(act), uint16(act), uint32(act), uint64(act),
				uintptr(act), float32(act), float64(act), json.Number(fmt.Sprint(act)),
			} {
				for _, expected := range []any{
					exp, int8(exp), int16(exp), int32(exp), int64(exp),
					uint(exp), uint8(exp), uint16(exp), uint32(exp), uint64(exp),
					uintptr(exp), float32(exp), float64(exp),
				} {
					name := fmt.Sprintf("%T and %T", actual, expected)
					t.Run(name, func(t *testing.T) {
						assertion := Greater(expected)
						if err := assertion.Assert(actual); err != nil {
							t.Errorf("%s: unexpected error: %s", name, err)
						}
					})
				}
			}
		})
		t.Run("other types", func(t *testing.T) {
			tests := map[string]struct {
				actual   any
				expected any
			}{
				"enum integer": {
					actual:   test.UserType_STAFF,
					expected: int(test.UserType_CUSTOMER),
				},
				"json.Number (int)": {
					actual:   json.Number("2"),
					expected: 1,
				},
				"json.Number (float)": {
					actual:   json.Number("3.14"),
					expected: 2,
				},
			}
			for name, tc := range tests {
				t.Run(name, func(t *testing.T) {
					assertion := Greater(tc.expected)
					if err := assertion.Assert(tc.actual); err != nil {
						t.Errorf("%s: unexpected error: %s", name, err)
					}
				})
			}
		})
	})
	t.Run("failure", func(t *testing.T) {
		t.Run("number", func(t *testing.T) {
			act := 2
			exp := 3
			for _, actual := range []any{
				act, int8(act), int16(act), int32(act), int64(act),
				uint(act), uint8(act), uint16(act), uint32(act), uint64(act),
				uintptr(act), float32(act), float64(act), json.Number(fmt.Sprint(act)), nil,
				json.Number("bad"),
			} {
				for _, expected := range []any{
					exp, int8(exp), int16(exp), int32(exp), int64(exp),
					uint(exp), uint8(exp), uint16(exp), uint32(exp), uint64(exp),
					uintptr(exp), float32(exp), float64(exp), "bad",
				} {
					name := fmt.Sprintf("%T and %T", actual, expected)
					t.Run(name, func(t *testing.T) {
						assertion := Greater(expected)
						if err := assertion.Assert(actual); err == nil {
							t.Errorf("%s: expected error but no error", name)
						}
					})
				}
			}
		})
		t.Run("other types", func(t *testing.T) {
			tests := map[string]struct {
				actual   any
				expected any
			}{
				"string": {
					actual:   "a",
					expected: "b",
				},
				"enum integer": {
					actual:   test.UserType_CUSTOMER,
					expected: int(test.UserType_STAFF),
				},
				"json.Number (int)": {
					actual:   1,
					expected: json.Number("2"),
				},
				"json.Number (float)": {
					actual:   1,
					expected: json.Number("3.14"),
				},
			}
			for name, tc := range tests {
				t.Run(name, func(t *testing.T) {
					assertion := Greater(tc.expected)
					if err := assertion.Assert(tc.actual); err == nil {
						t.Errorf("%s: expected error but no error", name)
					}
				})
			}
		})
	})
}

func TestGreaterOrEqual(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Run("number", func(t *testing.T) {
			act := 3
			exp := 2
			for _, actual := range []any{
				act, int8(act), int16(act), int32(act), int64(act),
				uint(act), uint8(act), uint16(act), uint32(act), uint64(act),
				uintptr(act), float32(act), float64(act), json.Number(fmt.Sprint(act)),
			} {
				for _, expected := range []any{
					exp, int8(exp), int16(exp), int32(exp), int64(exp),
					uint(exp), uint8(exp), uint16(exp), uint32(exp), uint64(exp),
					uintptr(exp), float32(exp), float64(exp),
				} {
					name := fmt.Sprintf("%T and %T", actual, expected)
					t.Run(name, func(t *testing.T) {
						assertion := GreaterOrEqual(expected)
						if err := assertion.Assert(actual); err != nil {
							t.Errorf("%s: unexpected error: %s", name, err)
						}
					})
				}
			}
		})
		t.Run("other types", func(t *testing.T) {
			tests := map[string]struct {
				actual   any
				expected any
			}{
				"enum integer": {
					actual:   test.UserType_STAFF,
					expected: int(test.UserType_CUSTOMER),
				},
				"json.Number (int)": {
					actual:   json.Number("2"),
					expected: 1,
				},
				"json.Number (float)": {
					actual:   json.Number("3.14"),
					expected: 2,
				},
			}
			for name, tc := range tests {
				t.Run(name, func(t *testing.T) {
					assertion := GreaterOrEqual(tc.expected)
					if err := assertion.Assert(tc.actual); err != nil {
						t.Errorf("%s: unexpected error: %s", name, err)
					}
				})
			}
		})
	})
	t.Run("failure", func(t *testing.T) {
		t.Run("number", func(t *testing.T) {
			act := 2
			exp := 3
			for _, actual := range []any{
				act, int8(act), int16(act), int32(act), int64(act),
				uint(act), uint8(act), uint16(act), uint32(act), uint64(act),
				uintptr(act), float32(act), float64(act), json.Number(fmt.Sprint(act)), nil,
				json.Number("bad"),
			} {
				for _, expected := range []any{
					exp, int8(exp), int16(exp), int32(exp), int64(exp),
					uint(exp), uint8(exp), uint16(exp), uint32(exp), uint64(exp),
					uintptr(exp), float32(exp), float64(exp), "bad",
				} {
					name := fmt.Sprintf("%T and %T", actual, expected)
					t.Run(name, func(t *testing.T) {
						assertion := GreaterOrEqual(expected)
						if err := assertion.Assert(actual); err == nil {
							t.Errorf("%s: expected error but no error", name)
						}
					})
				}
			}
		})
		t.Run("other types", func(t *testing.T) {
			tests := map[string]struct {
				actual   any
				expected any
			}{
				"string": {
					actual:   "a",
					expected: "b",
				},
				"enum integer": {
					actual:   test.UserType_CUSTOMER,
					expected: int(test.UserType_STAFF),
				},
				"json.Number (int)": {
					actual:   1,
					expected: json.Number("2"),
				},
				"json.Number (float)": {
					actual:   1,
					expected: json.Number("3.14"),
				},
			}
			for name, tc := range tests {
				t.Run(name, func(t *testing.T) {
					assertion := GreaterOrEqual(tc.expected)
					if err := assertion.Assert(tc.actual); err == nil {
						t.Errorf("%s: expected error but no error", name)
					}
				})
			}
		})
	})
}
