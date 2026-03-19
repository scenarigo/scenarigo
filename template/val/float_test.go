package val

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFloatType_Name(t *testing.T) {
	v := floatType
	if got, expect := v.Name(), "float"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestFloatType_NewValue(t *testing.T) {
	tests := map[string]struct {
		v           any
		expect      Value
		expectError string
	}{
		"float": {
			v:      1.1,
			expect: Float(1.1),
		},
		"not float": {
			v:           1,
			expectError: ErrUnsupportedType.Error(),
		},
		"nil": {
			v:           nil,
			expectError: ErrUnsupportedType.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := floatType.NewValue(test.v)
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

func TestFloatType_Convert(t *testing.T) {
	tests := map[string]struct {
		v           Value
		expect      Value
		expectError string
	}{
		"float": {
			v:      Float(1.1),
			expect: Float(1.1),
		},
		"any[float]": {
			v:      Any{1.1},
			expect: Float(1.1),
		},
		"int": {
			v:      Int(-1),
			expect: Float(-1.0),
		},
		"uint": {
			v:      Uint(1),
			expect: Float(1.0),
		},
		"string": {
			v:      String("-1.1"),
			expect: Float(-1.1),
		},
		"invalid string": {
			v:           String("test"),
			expectError: `strconv.ParseFloat: parsing "test": invalid syntax`,
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
			got, err := floatType.Convert(test.v)
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

func TestFloat_Type(t *testing.T) {
	v := Float(0)
	if got, expect := v.Type().Name(), floatType.Name(); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestFloat_GoValue(t *testing.T) {
	v := Float(1.1)
	got, expect := v.GoValue(), 1.1
	if diff := cmp.Diff(expect, got); diff != "" {
		t.Errorf("diff: (-want +got)\n%s", diff)
	}
}

func TestFloat_Neg(t *testing.T) {
	tests := map[string]struct {
		x           Float
		expect      any
		expectError string
	}{
		"1.1": {
			x:      Float(1.1),
			expect: Float(-1.1),
		},
		"-1.1": {
			x:      Float(-1.1),
			expect: Float(1.1),
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

func TestFloat_Equal(t *testing.T) {
	tests := map[string]struct {
		x           Float
		y           Value
		expect      any
		expectError string
	}{
		"1.1 == 1.1": {
			x:      Float(1.1),
			y:      Float(1.1),
			expect: Bool(true),
		},
		"1.1 == 2.2": {
			x:      Float(1.1),
			y:      Float(2.2),
			expect: Bool(false),
		},
		"1.1 == nil": {
			x:           Float(1.1),
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

func TestFloat_Compare(t *testing.T) {
	tests := map[string]struct {
		x           Float
		y           Value
		expect      any
		expectError string
	}{
		"1.1 == 1.1": {
			x:      Float(1.1),
			y:      Float(1.1),
			expect: Int(0),
		},
		"2.2 > 1.1": {
			x:      Float(2.2),
			y:      Float(1.1),
			expect: Int(1),
		},
		"1.1 < 2.2": {
			x:      Float(1.1),
			y:      Float(2.2),
			expect: Int(-1),
		},
		"nil is not float": {
			x:           Float(1.1),
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

func TestFloat_Add(t *testing.T) {
	tests := map[string]struct {
		x           Float
		y           Value
		expect      any
		expectError string
	}{
		"1.1 + 1.1": {
			x:      Float(1.1),
			y:      Float(1.1),
			expect: Float(2.2),
		},
		"nil is not float": {
			x:           Float(1.1),
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

func TestFloat_Sub(t *testing.T) {
	tests := map[string]struct {
		x           Float
		y           Value
		expect      any
		expectError string
	}{
		"1.1 - 1.1": {
			x:      Float(1.1),
			y:      Float(1.1),
			expect: Float(0.0),
		},
		"0.1 - 1.1": {
			x:      Float(0.1),
			y:      Float(1.1),
			expect: Float(-1.0),
		},
		"nil is not float": {
			x:           Float(1.1),
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

func TestFloat_Mul(t *testing.T) {
	tests := map[string]struct {
		x           Float
		y           Value
		expect      any
		expectError string
	}{
		"1.1 * 0.5": {
			x:      Float(1.1),
			y:      Float(0.5),
			expect: Float(0.55),
		},
		"nil is not float": {
			x:           Float(1.1),
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

func TestFloat_Div(t *testing.T) {
	tests := map[string]struct {
		x           Float
		y           Value
		expect      any
		expectError string
	}{
		"1.1 / 2.0": {
			x:      Float(1.1),
			y:      Float(2.0),
			expect: Float(0.55),
		},
		"1.1 / 0.0": {
			x:           Float(1.1),
			y:           Float(0.0),
			expectError: "division by 0",
		},
		"nil is not float": {
			x:           Float(1.1),
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
