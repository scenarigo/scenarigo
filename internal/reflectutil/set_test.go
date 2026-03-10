package reflectutil

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

func TestSet(t *testing.T) {
	type myStr string
	type myStruct struct {
		str string //nolint:unused
	}
	tests := map[string]struct {
		target reflect.Value
		v      reflect.Value
		expect any
		error  error
	}{
		"success": {
			target: reflect.New(reflect.TypeFor[string]()).Elem(),
			v:      reflect.ValueOf("test"),
			expect: "test",
		},
		"with type conversion": {
			target: reflect.New(reflect.TypeFor[string]()).Elem(),
			v:      reflect.ValueOf(myStr("test")),
			expect: "test",
		},
		"target is invalid": {
			target: reflect.Value{},
			v:      reflect.ValueOf("test"),
			error:  errors.New("can not set to invalid value"),
		},
		"v is invalid": {
			target: reflect.New(reflect.TypeFor[string]()).Elem(),
			v:      reflect.Value{},
			expect: "",
		},
		"can not set to unaddressable value": {
			target: reflect.ValueOf(""),
			v:      reflect.ValueOf("test"),
			error:  errors.New("can not set to unaddressable value"),
		},
		"can not set to unexported struct field": {
			target: reflect.New(reflect.TypeFor[myStruct]()).Elem().FieldByName("str"),
			v:      reflect.ValueOf("test"),
			error:  errors.New("can not set to unexported struct field"),
		},
		"not assignable": {
			target: reflect.New(reflect.TypeFor[int]()).Elem(),
			v:      reflect.ValueOf("test"),
			error:  errors.New("string is not assignable to int"),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := Set(test.target, test.v)
			if err != nil {
				if test.error == nil {
					t.Fatalf("unexpected error: %s", err)
				} else if got, expect := err.Error(), test.error.Error(); got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
			} else {
				if test.error != nil {
					t.Fatal("no error")
				}
				if diff := cmp.Diff(test.expect, test.target.Interface()); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
			}
		})
	}
}

func TestConvert(t *testing.T) {
	str := "test"
	tests := map[string]struct {
		target reflect.Type
		v      reflect.Value
		expect any
		ok     bool
		error  string
	}{
		"convert string to string": {
			target: reflect.TypeFor[string](),
			v:      reflect.ValueOf(str),
			expect: str,
			ok:     true,
		},
		"convert *string to string": {
			target: reflect.TypeFor[string](),
			v:      reflect.ValueOf(&str),
			expect: str,
			ok:     true,
		},
		"convert string to *string": {
			target: reflect.PointerTo(reflect.TypeFor[string]()),
			v:      reflect.ValueOf(str),
			expect: &str,
			ok:     true,
		},
		"convert (*string)(nil) to *string": {
			target: reflect.PointerTo(reflect.TypeFor[string]()),
			v:      reflect.ValueOf((*string)(nil)),
			expect: (*string)(nil),
			ok:     true,
		},
		"convert untyped nil to *string": {
			target: reflect.PointerTo(reflect.TypeFor[string]()),
			v:      reflect.ValueOf(nil),
			expect: (*string)(nil),
			ok:     true,
		},
		"convert string to Stringer": {
			target: reflect.TypeFor[stringer](),
			v:      reflect.ValueOf(str),
			expect: stringer(str),
			ok:     true,
		},
		"convert string to *Stringer": {
			target: reflect.PointerTo(reflect.TypeFor[stringer]()),
			v:      reflect.ValueOf(str),
			expect: (*stringer)(&str),
			ok:     true,
		},
		"failed to convert to untyped nil": {
			target: reflect.TypeOf(nil), //nolint:modernize // reflect.TypeFor cannot produce a nil reflect.Type
			v:      reflect.ValueOf(0),
			error:  "failed to convert to untyped nil",
		},
		"failed to convert string to int": {
			target: reflect.TypeFor[int](),
			v:      reflect.ValueOf(str),
			expect: str,
		},
		"failed to convert (*string)(nil) to string": {
			target: reflect.TypeFor[string](),
			v:      reflect.ValueOf((*string)(nil)),
			expect: (*string)(nil),
		},
		"failed to convert untyped nil to string": {
			target: reflect.TypeFor[string](),
			v:      reflect.ValueOf(nil),
			expect: nil,
		},
		"failed to convert int to string": {
			target: reflect.TypeFor[string](),
			v:      reflect.ValueOf(1),
			expect: 1,
		},
		"failed to convert uint to string": {
			target: reflect.TypeFor[string](),
			v:      reflect.ValueOf(uint(1)),
			expect: uint(1),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok, err := Convert(test.target, test.v)
			if err != nil {
				if test.error == "" {
					t.Fatalf("unexpected error: %s", err)
				} else if got, expect := err.Error(), test.error; got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
			} else {
				if test.error != "" {
					t.Fatal("no error")
				}
				if ok != test.ok {
					t.Fatalf("expect %t but got %t", test.ok, ok)
				}
				if got.IsValid() {
					if diff := cmp.Diff(test.expect, got.Interface()); diff != "" {
						t.Errorf("differs: (-want +got)\n%s", diff)
					}
				}
			}
		})
	}
}

func TestConvertInterface(t *testing.T) {
	str := "test"
	tests := map[string]struct {
		target reflect.Type
		v      any
		expect any
		ok     bool
		error  error
	}{
		"no need to convert": {
			target: reflect.TypeFor[string](),
			v:      str,
			expect: str,
			ok:     true,
		},
		"convert *string to string": {
			target: reflect.TypeFor[string](),
			v:      &str,
			expect: str,
			ok:     true,
		},
		"can't convert": {
			target: reflect.TypeFor[int](),
			v:      str,
			expect: str,
			ok:     false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok, err := ConvertInterface(test.target, test.v)
			if err != nil {
				if test.error == nil {
					t.Fatalf("unexpected error: %s", err)
				} else if got, expect := err.Error(), test.error.Error(); got != expect {
					t.Fatalf("expect %q but got %q", expect, got)
				}
			} else {
				if test.error != nil {
					t.Fatal("no error")
				}
				if ok != test.ok {
					t.Fatalf("expect %t but got %t", test.ok, ok)
				}
				if diff := cmp.Diff(test.expect, got); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
			}
		})
	}
}

type Stringer interface {
	String() string
}

type stringer string

func (s *stringer) String() string { return string(*s) }
