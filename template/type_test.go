package template

import (
	"testing"
	"time"

	"github.com/scenarigo/scenarigo/internal/testutil"
)

func TestTypeFunctions(t *testing.T) {
	tests := map[string]executeTestCase{
		"type(int)": {
			str:    "{{type(0)}}",
			expect: "int",
		},
		"type(int) == type(string)": {
			str:    `{{type(0) == type("")}}`,
			expect: false,
		},
		"int(string)": {
			str:    `{{int("-1")}}`,
			expect: int64(-1),
		},
		"uint(string)": {
			str:    `{{uint("1")}}`,
			expect: uint64(1),
		},
		"float(string)": {
			str:    `{{float("1.1")}}`,
			expect: 1.1,
		},
		"bool(*bool)": {
			str: `{{bool(v)}}`,
			data: map[string]interface{}{
				"v": testutil.ToPtr(true),
			},
			expect: true,
		},
		"string(int)": {
			str:    `{{string(-1)}}`,
			expect: "-1",
		},
		"bytes(string)": {
			str:    `{{bytes("test")}}`,
			expect: []byte("test"),
		},
		"time(string)": {
			str:    `{{time("2009-11-10T23:00:00Z")}}`,
			expect: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
		},
		"duration(string)": {
			str:    `{{duration("1s")}}`,
			expect: time.Second,
		},
	}
	runExecute(t, tests)
}
