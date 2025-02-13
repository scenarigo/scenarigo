package val

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/internal/testutil"
)

func TestUintType_Name(t *testing.T) {
	v := intType
	if got, expect := v.Name(), "int"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestUintType_NewValue(t *testing.T) {
	tests := map[string]struct {
		v           any
		expect      Value
		expectError string
	}{
		"uint": {
			v:      uint(1),
			expect: Uint(1),
		},
		"not uint": {
			v:           true,
			expectError: ErrUnsupportedType.Error(),
		},
		"nil": {
			v:           nil,
			expectError: ErrUnsupportedType.Error(),
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			got, err := uintType.NewValue(test.v)
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

func TestUintType_Convert(t *testing.T) {
	tests := map[string]struct {
		v           Value
		expect      Value
		expectError string
	}{
		"uint": {
			v:      Uint(1),
			expect: Uint(1),
		},
		"any[*uint]": {
			v:      Any{testutil.ToPtr(uint(1))},
			expect: Uint(1),
		},
		"int": {
			v:      Int(1),
			expect: Uint(1),
		},
		"int underflows uint": {
			v:           Int(-1),
			expectError: "can't convert -1 to uint",
		},
		"float": {
			v:      Float(1.9),
			expect: Uint(1),
		},
		"float underflows uint": {
			v:           Float(-1.1),
			expectError: "can't convert -1.1 to uint",
		},
		"float overflows uint": {
			v:           Float(math.MaxUint64 + 10000),
			expectError: "18446744073709560000 overflows uint",
		},
		"string": {
			v:      String("1"),
			expect: Uint(1),
		},
		"invalid string": {
			v:           String("-1"),
			expectError: `strconv.ParseUint: parsing "-1": invalid syntax`,
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
		test := test
		t.Run(name, func(t *testing.T) {
			got, err := uintType.Convert(test.v)
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

func TestUint_Type(t *testing.T) {
	v := Uint(0)
	if got, expect := v.Type().Name(), uintType.Name(); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestUint_GoValue(t *testing.T) {
	v := Uint(1)
	got, expect := v.GoValue(), uint64(1)
	if diff := cmp.Diff(expect, got); diff != "" {
		t.Errorf("diff: (-want +got)\n%s", diff)
	}
}

func TestUint_Equal(t *testing.T) {
	tests := map[string]struct {
		x           Uint
		y           Value
		expect      interface{}
		expectError string
	}{
		"1 == 1": {
			x:      Uint(1),
			y:      Uint(1),
			expect: Bool(true),
		},
		"1 == 2": {
			x:      Uint(1),
			y:      Uint(2),
			expect: Bool(false),
		},
		"1 == nil": {
			x:           Uint(1),
			y:           Nil{},
			expect:      Bool(false),
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		test := test
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

func TestUint_Compare(t *testing.T) {
	tests := map[string]struct {
		x           Uint
		y           Value
		expect      interface{}
		expectError string
	}{
		"1 == 1": {
			x:      Uint(1),
			y:      Uint(1),
			expect: Int(0),
		},
		"2 > 1": {
			x:      Uint(2),
			y:      Uint(1),
			expect: Int(1),
		},
		"1 < 2": {
			x:      Uint(1),
			y:      Uint(2),
			expect: Int(-1),
		},
		"nil is not uint": {
			x:           Uint(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		test := test
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

func TestUint_Add(t *testing.T) {
	tests := map[string]struct {
		x           Uint
		y           Value
		expect      interface{}
		expectError string
	}{
		"1 + 1": {
			x:      Uint(1),
			y:      Uint(1),
			expect: Uint(2),
		},
		"max uint": {
			x:      Uint(math.MaxUint64 - 1),
			y:      Uint(1),
			expect: Uint(math.MaxUint64),
		},
		"overflow": {
			x:           Uint(math.MaxUint64),
			y:           Uint(1),
			expectError: fmt.Sprintf("%d + 1 overflows uint", uint64(math.MaxUint64)),
		},
		"nil is not uint": {
			x:           Uint(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		test := test
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

func TestUint_Sub(t *testing.T) {
	tests := map[string]struct {
		x           Uint
		y           Value
		expect      interface{}
		expectError string
	}{
		"1 - 1": {
			x:      Uint(1),
			y:      Uint(1),
			expect: Uint(0),
		},
		"0 - 1": {
			x:           Uint(0),
			y:           Uint(1),
			expectError: "0 - 1 overflows uint",
		},
		"nil is not uint": {
			x:           Uint(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		test := test
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

func TestUint_Mul(t *testing.T) {
	tests := map[string]struct {
		x           Uint
		y           Value
		expect      interface{}
		expectError string
	}{
		"2 * 3": {
			x:      Uint(2),
			y:      Uint(3),
			expect: Uint(6),
		},
		"max uint": {
			x:      Uint(math.MaxUint64),
			y:      Uint(1),
			expect: Uint(math.MaxUint64),
		},
		"overflow": {
			x:           Uint(math.MaxUint64),
			y:           Uint(2),
			expectError: fmt.Sprintf("%d * 2 overflows uint", uint64(math.MaxUint64)),
		},
		"nil is not uint": {
			x:           Uint(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		test := test
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

func TestUint_Div(t *testing.T) {
	tests := map[string]struct {
		x           Uint
		y           Value
		expect      interface{}
		expectError string
	}{
		"3 / 2": {
			x:      Uint(3),
			y:      Uint(2),
			expect: Uint(1),
		},
		"1 / 0": {
			x:           Uint(1),
			y:           Uint(0),
			expectError: "division by 0",
		},
		"nil is not uint": {
			x:           Uint(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		test := test
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

func TestUint_Mod(t *testing.T) {
	tests := map[string]struct {
		x           Uint
		y           Value
		expect      interface{}
		expectError string
	}{
		"5 % 3": {
			x:      Uint(5),
			y:      Uint(3),
			expect: Uint(2),
		},
		"1 % 0": {
			x:           Uint(1),
			y:           Uint(0),
			expectError: "division by 0",
		},
		"nil is not uint": {
			x:           Uint(1),
			y:           Nil{},
			expectError: ErrOperationNotDefined.Error(),
		},
	}
	for name, test := range tests {
		test := test
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
