package val

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/internal/testutil"
)

func TestNewValue(t *testing.T) {
	now := time.Now()
	tests := map[string]struct {
		v      any
		expect Value
	}{
		"int": {
			v:      1,
			expect: Int(1),
		},
		"uint": {
			v:      uint(1),
			expect: Uint(1),
		},
		"float": {
			v:      1.1,
			expect: Float(1.1),
		},
		"bool": {
			v:      true,
			expect: Bool(true),
		},
		"string": {
			v:      "test",
			expect: String("test"),
		},
		"bytes": {
			v:      []byte("test"),
			expect: Bytes([]byte("test")),
		},
		"time": {
			v:      now,
			expect: Time(now),
		},
		"duration": {
			v:      time.Minute,
			expect: Duration(time.Minute),
		},
		"(*int)(nil)": {
			v:      (*int)(nil),
			expect: Nil{(*int)(nil)},
		},
		"*int": {
			v:      testutil.ToPtr(1),
			expect: Any{testutil.ToPtr(1)},
		},
		"Value": {
			v:      Int(0),
			expect: Int(0),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := NewValue(test.v)
			if diff := cmp.Diff(test.expect, got, cmp.AllowUnexported(Time{}, time.Location{}, Nil{}, Any{})); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

var _ LogicalValue = Bool(false)

var (
	_ Negator = Int(0)
	_ Negator = Float(0)
	_ Negator = Duration(0)
)

var (
	_ Adder = Int(0)
	_ Adder = Uint(0)
	_ Adder = Float(0)
	_ Adder = String("")
	_ Adder = Bytes("")
	_ Adder = Time(time.Now())
	_ Adder = Duration(0)
)

var (
	_ Subtractor = Int(0)
	_ Subtractor = Uint(0)
	_ Subtractor = Float(0)
	_ Subtractor = Time(time.Now())
	_ Subtractor = Duration(0)
)

var (
	_ Multiplier = Int(0)
	_ Multiplier = Uint(0)
	_ Multiplier = Float(0)
)

var (
	_ Divider = Int(0)
	_ Divider = Uint(0)
	_ Divider = Float(0)
)

var (
	_ Modder = Int(0)
	_ Modder = Uint(0)
)

var (
	_ Equaler = Int(0)
	_ Equaler = Uint(0)
	_ Equaler = Float(0)
	_ Equaler = Bool(false)
	_ Equaler = String("")
	_ Equaler = Bytes("")
	_ Equaler = Time(time.Now())
	_ Equaler = Duration(0)
	_ Equaler = Any{}
)

var (
	_ Comparer = Int(0)
	_ Comparer = Uint(0)
	_ Comparer = Float(0)
	_ Comparer = String("")
	_ Comparer = Bytes("")
	_ Comparer = Time(time.Now())
	_ Comparer = Duration(0)
)
