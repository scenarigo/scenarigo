package val

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestAnyType_Name(t *testing.T) {
	if got, expect := createAnyType(nil).Name(), "any"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
	if got, expect := createAnyType(false).Name(), "any[bool]"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestAnyType_NewValue(t *testing.T) {
	tests := map[string]struct {
		v           any
		expect      Value
		expectError string
	}{
		"bool": {
			v:      true,
			expect: Any{true},
		},
		"nil": {
			v:      nil,
			expect: Any{},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := anyType.NewValue(test.v)
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
			if diff := cmp.Diff(test.expect, got, cmp.AllowUnexported(Any{})); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestAnyType_Convert(t *testing.T) {
	tests := map[string]struct {
		v           Value
		expect      Value
		expectError string
	}{
		"true": {
			v:      Bool(true),
			expect: Any{true},
		},
		"nil": {
			v:      nil,
			expect: Any{nil},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := anyType.Convert(test.v)
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
			if diff := cmp.Diff(test.expect, got, cmp.AllowUnexported(Any{})); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestAny_Type(t *testing.T) {
	v := Any{false}
	if got, expect := v.Type().Name(), createAnyType(false).Name(); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestAny_GoValue(t *testing.T) {
	v := Any{v: 1}
	if got, expect := v.GoValue(), 1; got != expect {
		t.Errorf("expect %v but got %v", expect, got)
	}
}

func TestAny_Equal(t *testing.T) {
	tests := map[string]struct {
		x           Any
		y           Value
		expect      interface{}
		expectError string
	}{
		"1 == 1": {
			x:      Any{1},
			y:      Any{1},
			expect: Bool(true),
		},
		"1 == 2": {
			x:      Any{1},
			y:      Any{2},
			expect: Bool(false),
		},
		`1 == "1"`: {
			x:      Any{1},
			y:      Any{"1"},
			expect: Bool(false),
		},
		`1 == nil`: {
			x:      Any{1},
			y:      Any{nil},
			expect: Bool(false),
		},
		`nil == 1`: {
			x:           Any{nil},
			y:           Any{1},
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

func TestAny_Size(t *testing.T) {
	s := []int{0, 1}
	tests := map[string]struct {
		v           Any
		expect      Value
		expectError string
	}{
		"array": {
			v:      Any{[1]int{0}},
			expect: Int(1),
		},
		"slice": {
			v:      Any{s},
			expect: Int(2),
		},
		"map": {
			v:      Any{map[string]string{"foo": "bar"}},
			expect: Int(1),
		},
		"*slice": {
			v:      Any{&s},
			expect: Int(2),
		},
		"int": {
			v:           Any{0},
			expectError: "size(any[int]) is not defined",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := test.v.Size()
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
			if diff := cmp.Diff(test.expect, got, cmp.AllowUnexported(Any{})); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}
