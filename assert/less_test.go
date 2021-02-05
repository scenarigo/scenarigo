package assert

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/zoncoen/query-go"
	"github.com/zoncoen/scenarigo/testdata/gen/pb/test"
)

func TestLess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Run("builtin number", func(t *testing.T) {
			v := 2
			v2 := 3
			for _, expected := range []interface{}{
				int(v), int8(v), int16(v), int32(v), int64(v),
				uint(v), uint8(v), uint16(v), uint32(v), uint64(v),
				float32(v), float64(v),
			} {
				for _, ok := range []interface{}{
					int(v2), int8(v2), int16(v2), int32(v2), int64(v2),
					uint(v2), uint8(v2), uint16(v2), uint32(v2), uint64(v2),
					float32(v2), float64(v2),
				} {
					name := fmt.Sprintf("%T and %T", expected, ok)
					t.Run(name, func(t *testing.T) {
						assertion := Less(query.New(), expected)
						if err := assertion.Assert(ok); err != nil {
							t.Errorf("%s: unexpected error: %s", name, err)
						}
					})
				}
			}
		})
		t.Run("other types", func(t *testing.T) {
			tests := map[string]struct {
				expected interface{}
				ok       interface{}
			}{
				"string": {
					expected: "b",
					ok:       "c",
				},
				"enum integer": {
					expected: int(test.UserType_CUSTOMER),
					ok:       test.UserType_STAFF,
				},
				"json.Number (int)": {
					expected: 1,
					ok:       json.Number("2"),
				},
				"json.Number (float)": {
					expected: 1,
					ok:       json.Number("3.14"),
				},
			}
			for name, tc := range tests {
				tc := tc
				t.Run(name, func(t *testing.T) {
					assertion := Less(query.New(), tc.expected)
					if err := assertion.Assert(tc.ok); err != nil {
						t.Errorf("%s: unexpected error: %s", name, err)
					}
				})
			}
		})
	})
	t.Run("failure", func(t *testing.T) {
		t.Run("builtin number", func(t *testing.T) {
			v := 2
			v2 := 1
			for _, expected := range []interface{}{
				int(v), int8(v), int16(v), int32(v), int64(v),
				uint(v), uint8(v), uint16(v), uint32(v), uint64(v),
				float32(v), float64(v),
			} {
				for _, ng := range []interface{}{
					int(v2), int8(v2), int16(v2), int32(v2), int64(v2),
					uint(v2), uint8(v2), uint16(v2), uint32(v2), uint64(v2),
					float32(v2), float64(v2),
				} {
					name := fmt.Sprintf("%T and %T", expected, ng)
					t.Run(name, func(t *testing.T) {
						assertion := Less(query.New(), expected)
						if err := assertion.Assert(ng); err == nil {
							t.Errorf("%s: expected error but no error", name)
						}
					})
				}
			}
		})
		t.Run("other types", func(t *testing.T) {
			tests := map[string]struct {
				expected interface{}
				ng       interface{}
			}{
				"string (equal)": {
					expected: "a",
					ng:       "a",
				},
				"string": {
					expected: "b",
					ng:       "a",
				},
				"enum integer": {
					expected: int(test.UserType_STAFF),
					ng:       test.UserType_CUSTOMER,
				},
				"json.Number (int)": {
					expected: json.Number("2"),
					ng:       1,
				},
				"json.Number (float)": {
					expected: json.Number("3.14"),
					ng:       2,
				},
			}
			for name, tc := range tests {
				tc := tc
				t.Run(name, func(t *testing.T) {
					assertion := Less(query.New(), tc.expected)
					if err := assertion.Assert(tc.ng); err == nil {
						t.Errorf("%s: expected error but no error", name)
					}
				})
			}
		})
	})
}

func TestLessOrEqual(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Run("builtin number", func(t *testing.T) {
			v := 2
			v2 := 3
			for _, expected := range []interface{}{
				int(v), int8(v), int16(v), int32(v), int64(v),
				uint(v), uint8(v), uint16(v), uint32(v), uint64(v),
				float32(v), float64(v),
			} {
				for _, ok := range []interface{}{
					int(v2), int8(v2), int16(v2), int32(v2), int64(v2),
					uint(v2), uint8(v2), uint16(v2), uint32(v2), uint64(v2),
					float32(v2), float64(v2),
				} {
					name := fmt.Sprintf("%T and %T", expected, ok)
					t.Run(name, func(t *testing.T) {
						assertion := LessOrEqual(query.New(), expected)
						if err := assertion.Assert(ok); err != nil {
							t.Errorf("%s: unexpected error: %s", name, err)
						}
					})
				}
			}
		})
		t.Run("other types", func(t *testing.T) {
			tests := map[string]struct {
				expected interface{}
				ok       interface{}
			}{
				"string (equal)": {
					expected: "a",
					ok:       "a",
				},
				"string": {
					expected: "a",
					ok:       "b",
				},
				"enum integer": {
					expected: int(test.UserType_CUSTOMER),
					ok:       test.UserType_STAFF,
				},
				"json.Number (int)": {
					expected: 1,
					ok:       json.Number("2"),
				},
				"json.Number (float)": {
					expected: 2,
					ok:       json.Number("3.14"),
				},
			}
			for name, tc := range tests {
				tc := tc
				t.Run(name, func(t *testing.T) {
					assertion := LessOrEqual(query.New(), tc.expected)
					if err := assertion.Assert(tc.ok); err != nil {
						t.Errorf("%s: unexpected error: %s", name, err)
					}
				})
			}
		})
	})
	t.Run("failure", func(t *testing.T) {
		t.Run("builtin number", func(t *testing.T) {
			v := 3
			v2 := 2
			for _, expected := range []interface{}{
				int(v), int8(v), int16(v), int32(v), int64(v),
				uint(v), uint8(v), uint16(v), uint32(v), uint64(v),
				float32(v), float64(v),
			} {
				for _, ng := range []interface{}{
					int(v2), int8(v2), int16(v2), int32(v2), int64(v2),
					uint(v2), uint8(v2), uint16(v2), uint32(v2), uint64(v2),
					float32(v2), float64(v2),
				} {
					name := fmt.Sprintf("%T and %T", expected, ng)
					t.Run(name, func(t *testing.T) {
						assertion := LessOrEqual(query.New(), expected)
						if err := assertion.Assert(ng); err == nil {
							t.Errorf("%s: expected error but no error", name)
						}
					})
				}
			}
		})
		t.Run("other types", func(t *testing.T) {
			tests := map[string]struct {
				expected interface{}
				ng       interface{}
			}{
				"string": {
					expected: "b",
					ng:       "a",
				},
				"enum integer": {
					expected: int(test.UserType_STAFF),
					ng:       test.UserType_CUSTOMER,
				},
				"json.Number (int)": {
					expected: json.Number("2"),
					ng:       1,
				},
				"json.Number (float)": {
					expected: json.Number("3.14"),
					ng:       2,
				},
			}
			for name, tc := range tests {
				tc := tc
				t.Run(name, func(t *testing.T) {
					assertion := LessOrEqual(query.New(), tc.expected)
					if err := assertion.Assert(tc.ng); err == nil {
						t.Errorf("%s: expected error but no error", name)
					}
				})
			}
		})
	})
}
