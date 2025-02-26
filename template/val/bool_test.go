package val

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBoolType_Name(t *testing.T) {
	v := boolType
	if got, expect := v.Name(), "bool"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestBoolType_NewValue(t *testing.T) {
	tests := map[string]struct {
		v           any
		expect      Value
		expectError string
	}{
		"true": {
			v:      true,
			expect: Bool(true),
		},
		"false": {
			v:      false,
			expect: Bool(false),
		},
		"not bool": {
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
			got, err := boolType.NewValue(test.v)
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

func TestBoolType_Convert(t *testing.T) {
	tests := map[string]struct {
		v           Value
		expect      Value
		expectError string
	}{
		"bool(true)": {
			v:      Bool(true),
			expect: Bool(true),
		},
		"bool(false)": {
			v:      Bool(false),
			expect: Bool(false),
		},
		"any[bool]": {
			v:      Any{true},
			expect: Bool(true),
		},
		"not bool": {
			v:           Int(1),
			expectError: ErrUnsupportedType.Error(),
		},
		"nil": {
			v:           nil,
			expectError: ErrUnsupportedType.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := boolType.Convert(test.v)
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

func TestBool_Type(t *testing.T) {
	v := Bool(true)
	if got, expect := v.Type().Name(), boolType.Name(); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestBool_GoValue(t *testing.T) {
	v := Bool(true)
	if got, expect := v.GoValue(), true; got != expect {
		t.Errorf("expect %v but got %v", expect, got)
	}
}

func TestBool_Equal(t *testing.T) {
	tests := map[string]struct {
		x           Bool
		y           Value
		expect      interface{}
		expectError string
	}{
		"true == true": {
			x:      Bool(true),
			y:      Bool(true),
			expect: Bool(true),
		},
		"true == false": {
			x:      Bool(true),
			y:      Bool(false),
			expect: Bool(false),
		},
		"true == nil": {
			x:           Bool(true),
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

func TestBool_IsTruthy(t *testing.T) {
	tests := map[string]struct {
		x      Bool
		expect bool
	}{
		"true": {
			x:      Bool(true),
			expect: true,
		},
		"false": {
			x:      Bool(false),
			expect: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got, expect := test.x.IsTruthy(), test.expect; got != expect {
				t.Errorf("expect %v but got %v", expect, got)
			}
		})
	}
}
