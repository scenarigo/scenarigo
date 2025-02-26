package val

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBytesType_Name(t *testing.T) {
	v := bytesType
	if got, expect := v.Name(), "bytes"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestBytesType_NewValue(t *testing.T) {
	tests := map[string]struct {
		v           any
		expect      Value
		expectError string
	}{
		"[]byte": {
			v:      []byte("test"),
			expect: Bytes([]byte("test")),
		},
		"[]uint8": {
			v:      []uint8{1},
			expect: Bytes([]byte{byte(1)}),
		},
		"string": {
			v:           "string",
			expectError: ErrUnsupportedType.Error(),
		},
		"nil": {
			v:           nil,
			expectError: ErrUnsupportedType.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := bytesType.NewValue(test.v)
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

func TestBytesType_Convert(t *testing.T) {
	tests := map[string]struct {
		v           Value
		expect      Value
		expectError string
	}{
		"bytes": {
			v:      Bytes([]byte("test")),
			expect: Bytes([]byte("test")),
		},
		"any[[]uint8]": {
			v:      Any{[]uint8{1}},
			expect: Bytes([]byte{byte(1)}),
		},
		"string": {
			v:      String("test"),
			expect: Bytes([]byte("test")),
		},
		"int": {
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
			got, err := bytesType.Convert(test.v)
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

func TestBytes_Type(t *testing.T) {
	v := Bytes(nil)
	if got, expect := v.Type().Name(), bytesType.Name(); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestBytes_GoValue(t *testing.T) {
	v := Bytes([]byte("foo"))
	got, expect := v.GoValue(), []byte("foo")
	if diff := cmp.Diff(expect, got); diff != "" {
		t.Errorf("diff: (-want +got)\n%s", diff)
	}
}

func TestBytes_Equal(t *testing.T) {
	tests := map[string]struct {
		x           Bytes
		y           Value
		expect      interface{}
		expectError string
	}{
		"foo == foo": {
			x:      Bytes([]byte("foo")),
			y:      Bytes([]byte("foo")),
			expect: Bool(true),
		},
		"foo == bar": {
			x:      Bytes([]byte("foo")),
			y:      Bytes([]byte("bar")),
			expect: Bool(false),
		},
		"foo == nil": {
			x:           Bytes([]byte("foo")),
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

func TestBytes_Compare(t *testing.T) {
	tests := map[string]struct {
		x           Bytes
		y           Value
		expect      interface{}
		expectError string
	}{
		"foo == foo": {
			x:      Bytes([]byte("foo")),
			y:      Bytes([]byte("foo")),
			expect: Int(0),
		},
		"foo > bar": {
			x:      Bytes([]byte("foo")),
			y:      Bytes([]byte("bar")),
			expect: Int(1),
		},
		"bar < foo": {
			x:      Bytes([]byte("bar")),
			y:      Bytes([]byte("foo")),
			expect: Int(-1),
		},
		"nil is not bytes": {
			x:           Bytes([]byte("foo")),
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

func TestBytes_Add(t *testing.T) {
	tests := map[string]struct {
		x           Bytes
		y           Value
		expect      interface{}
		expectError string
	}{
		"foo + bar": {
			x:      Bytes([]byte("foo")),
			y:      Bytes([]byte("bar")),
			expect: Bytes([]byte("foobar")),
		},
		"nil is not bytes": {
			x:           Bytes([]byte("foo")),
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

func TestBytes_Size(t *testing.T) {
	tests := map[string]struct {
		v           Bytes
		expect      Value
		expectError string
	}{
		"unibyte": {
			v:      Bytes([]byte("test")),
			expect: Int(4),
		},
		"multibyte": {
			v:      Bytes([]byte("テスト")),
			expect: Int(9),
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
