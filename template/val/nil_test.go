package val

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/internal/testutil"
)

func TestNilType_Name(t *testing.T) {
	v := nilType
	if got, expect := v.Name(), "nil"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestNilType_NewValue(t *testing.T) {
	tests := map[string]struct {
		v           any
		expect      Value
		expectError string
	}{
		"nil": {
			v:      nil,
			expect: Nil{},
		},
		"typed nil": {
			v:      (*int)(nil),
			expect: Nil{(*int)(nil)},
		},
		"not nil": {
			v:           testutil.ToPtr(1),
			expectError: ErrUnsupportedType.Error(),
		},
		"int": {
			v:           1,
			expectError: ErrUnsupportedType.Error(),
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			got, err := nilType.NewValue(test.v)
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
			if diff := cmp.Diff(test.expect, got, cmp.AllowUnexported(Nil{})); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestNilType_Convert(t *testing.T) {
	tests := map[string]struct {
		v           Value
		expect      Value
		expectError string
	}{
		"nil Value": {
			v:      nil,
			expect: Nil{},
		},
		"nil": {
			v:      Nil{(*int)(nil)},
			expect: Nil{(*int)(nil)},
		},
		"int": {
			v:           Int(1),
			expectError: ErrUnsupportedType.Error(),
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			got, err := nilType.Convert(test.v)
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
			if diff := cmp.Diff(test.expect, got, cmp.AllowUnexported(Nil{})); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestNil_Type(t *testing.T) {
	v := Nil{}
	if got, expect := v.Type().Name(), nilType.Name(); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestNil_GoValue(t *testing.T) {
	v := Nil{(*int)(nil)}
	if got, expect := v.GoValue(), (*int)(nil); got != expect {
		t.Errorf("expect %v but got %v", expect, got)
	}
}

func TestNil_Equal(t *testing.T) {
	tests := map[string]struct {
		x           Nil
		y           Value
		expect      interface{}
		expectError string
	}{
		"(*int)(nil) == (*int)(nil)": {
			x:      Nil{(*int)(nil)},
			y:      Nil{(*int)(nil)},
			expect: Bool(true),
		},
		"(*int)(nil) == (*bool)(nil)": {
			x:      Nil{(*int)(nil)},
			y:      Nil{(*bool)(nil)},
			expect: Bool(true),
		},
		"(*int)(nil) == nil": {
			x:      Nil{(*int)(nil)},
			y:      Nil{},
			expect: Bool(true),
		},
		"nil == nil": {
			x:      Nil{},
			y:      Nil{},
			expect: Bool(true),
		},
		"(*int)(nil) == true": {
			x:           Nil{(*int)(nil)},
			y:           Bool(true),
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
