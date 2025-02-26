package val

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/internal/testutil"
)

func TestIntType_Name(t *testing.T) {
	v := intType
	if got, expect := v.Name(), "int"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestIntType_NewValue(t *testing.T) {
	tests := map[string]struct {
		v           any
		expect      Value
		expectError string
	}{
		"int": {
			v:      1,
			expect: Int(1),
		},
		"not int": {
			v:           true,
			expectError: ErrUnsupportedType.Error(),
		},
		"nil": {
			v:           nil,
			expectError: ErrUnsupportedType.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := intType.NewValue(test.v)
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
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestIntType_Convert(t *testing.T) {
	tests := map[string]struct {
		v           Value
		expect      Value
		expectError string
	}{
		"int": {
			v:      Int(1),
			expect: Int(1),
		},
		"any[*int]": {
			v:      Any{testutil.ToPtr(1)},
			expect: Int(1),
		},
		"uint": {
			v:      Uint(1),
			expect: Int(1),
		},
		"uint overflows int": {
			v:           Uint(math.MaxInt64 + 1),
			expectError: "9223372036854775808 overflows int",
		},
		"float": {
			v:      Float(2.9),
			expect: Int(2),
		},
		"float underflows int": {
			v:           Float(math.MinInt64 - 10000),
			expectError: "-9223372036854786000 overflows int",
		},
		"float overflows int": {
			v:           Float(math.MaxInt64 + 10000),
			expectError: "9223372036854786000 overflows int",
		},
		"string": {
			v:      String("123"),
			expect: Int(123),
		},
		"invalid string": {
			v:           String("abc"),
			expectError: `strconv.ParseInt: parsing "abc": invalid syntax`,
		},
		"bool": {
			v:           Bool(false),
			expectError: ErrUnsupportedType.Error(),
		},
		"nil": {
			v:           nil,
			expectError: ErrUnsupportedType.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := intType.Convert(test.v)
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
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestInt_Type(t *testing.T) {
	v := Int(0)
	if got, expect := v.Type().Name(), intType.Name(); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestInt_GoValue(t *testing.T) {
	v := Int(1)
	got, expect := v.GoValue(), int64(1)
	if diff := cmp.Diff(expect, got); diff != "" {
		t.Errorf("diff: (-want +got)\n%s", diff)
	}
}

func TestInt_Neg(t *testing.T) {
	tests := map[string]struct {
		x           Int
		expect      interface{}
		expectError string
	}{
		"1": {
			x:      Int(1),
			expect: Int(-1),
		},
		"max int": {
			x:      Int(math.MaxInt64),
			expect: Int(-math.MaxInt64),
		},
		"-1": {
			x:      Int(-1),
			expect: Int(1),
		},
		"min int": {
			x:           Int(math.MinInt64),
			expectError: fmt.Sprintf("-(%d) overflows int", math.MinInt64),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := test.x.Neg()
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
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestInt_Equal(t *testing.T) {
	tests := map[string]struct {
		x           Int
		y           Value
		expect      interface{}
		expectError string
	}{
		"1 == 1": {
			x:      Int(1),
			y:      Int(1),
			expect: Bool(true),
		},
		"1 == 2": {
			x:      Int(1),
			y:      Int(2),
			expect: Bool(false),
		},
		"1 == nil": {
			x:           Int(1),
			y:           Nil{},
			expect:      Bool(false),
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := test.x.Equal(test.y)
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
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestInt_Compare(t *testing.T) {
	tests := map[string]struct {
		x           Int
		y           Value
		expect      interface{}
		expectError string
	}{
		"1 == 1": {
			x:      Int(1),
			y:      Int(1),
			expect: Int(0),
		},
		"2 > 1": {
			x:      Int(2),
			y:      Int(1),
			expect: Int(1),
		},
		"1 < 2": {
			x:      Int(1),
			y:      Int(2),
			expect: Int(-1),
		},
		"nil is not int": {
			x:           Int(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := test.x.Compare(test.y)
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
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestInt_Add(t *testing.T) {
	tests := map[string]struct {
		x           Int
		y           Value
		expect      interface{}
		expectError string
	}{
		"1 + 1": {
			x:      Int(1),
			y:      Int(1),
			expect: Int(2),
		},
		"max int": {
			x:      Int(math.MaxInt64 - 1),
			y:      Int(1),
			expect: Int(math.MaxInt64),
		},
		"overflow": {
			x:           Int(math.MaxInt64),
			y:           Int(1),
			expectError: "9223372036854775807 + 1 overflows int",
		},
		"min int": {
			x:      Int(math.MinInt64 + 1),
			y:      Int(-1),
			expect: Int(math.MinInt64),
		},
		"underflow": {
			x:           Int(math.MinInt64),
			y:           Int(-1),
			expectError: "-9223372036854775808 + -1 overflows int",
		},
		"nil is not int": {
			x:           Int(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := test.x.Add(test.y)
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
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestInt_Sub(t *testing.T) {
	tests := map[string]struct {
		x           Int
		y           Value
		expect      interface{}
		expectError string
	}{
		"1 - 1": {
			x:      Int(1),
			y:      Int(1),
			expect: Int(0),
		},
		"max int": {
			x:      Int(math.MaxInt64 - 1),
			y:      Int(-1),
			expect: Int(math.MaxInt64),
		},
		"overflow": {
			x:           Int(math.MaxInt64),
			y:           Int(-1),
			expectError: "9223372036854775807 - -1 overflows int",
		},
		"min int": {
			x:      Int(math.MinInt64 + 1),
			y:      Int(1),
			expect: Int(math.MinInt64),
		},
		"underflow": {
			x:           Int(math.MinInt64),
			y:           Int(1),
			expectError: "-9223372036854775808 - 1 overflows int",
		},
		"nil is not int": {
			x:           Int(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := test.x.Sub(test.y)
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
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestInt_Mul(t *testing.T) {
	tests := map[string]struct {
		x           Int
		y           Value
		expect      interface{}
		expectError string
	}{
		"2 * 3": {
			x:      Int(2),
			y:      Int(3),
			expect: Int(6),
		},
		"max int": {
			x:      Int(math.MaxInt64),
			y:      Int(1),
			expect: Int(math.MaxInt64),
		},
		"overflow": {
			x:           Int(math.MaxInt64),
			y:           Int(2),
			expectError: "9223372036854775807 * 2 overflows int",
		},
		"min int": {
			x:      Int(math.MinInt64),
			y:      Int(1),
			expect: Int(math.MinInt64),
		},
		"underflow": {
			x:           Int(math.MinInt64),
			y:           Int(2),
			expectError: "-9223372036854775808 * 2 overflows int",
		},
		"nil is not int": {
			x:           Int(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := test.x.Mul(test.y)
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
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestInt_Div(t *testing.T) {
	tests := map[string]struct {
		x           Int
		y           Value
		expect      interface{}
		expectError string
	}{
		"3 / 2": {
			x:      Int(3),
			y:      Int(2),
			expect: Int(1),
		},
		"1 / 0": {
			x:           Int(1),
			y:           Int(0),
			expectError: "division by 0",
		},
		"nil is not int": {
			x:           Int(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := test.x.Div(test.y)
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
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestInt_Mod(t *testing.T) {
	tests := map[string]struct {
		x           Int
		y           Value
		expect      interface{}
		expectError string
	}{
		"5 % 3": {
			x:      Int(5),
			y:      Int(3),
			expect: Int(2),
		},
		"1 % 0": {
			x:           Int(1),
			y:           Int(0),
			expectError: "division by 0",
		},
		"nil is not int": {
			x:           Int(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := test.x.Mod(test.y)
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
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}
