package val

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/internal/testutil"
)

func TestStringType_Name(t *testing.T) {
	v := stringType
	if got, expect := v.Name(), "string"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestStringType_NewValue(t *testing.T) {
	tests := map[string]struct {
		v           any
		expect      Value
		expectError string
	}{
		"string": {
			v:      "test",
			expect: String("test"),
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
			got, err := stringType.NewValue(test.v)
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

func TestStringType_Convert(t *testing.T) {
	tests := map[string]struct {
		v           Value
		expect      Value
		expectError string
	}{
		"string": {
			v:      String("test"),
			expect: String("test"),
		},
		"any[*string]": {
			v:      Any{testutil.ToPtr("test")},
			expect: String("test"),
		},
		"int": {
			v:      Int(-1),
			expect: String("-1"),
		},
		"uint": {
			v:      Uint(1),
			expect: String("1"),
		},
		"float": {
			v:      Float(1.2),
			expect: String("1.2"),
		},
		"[]byte": {
			v:      Bytes([]byte("test")),
			expect: String("test"),
		},
		"time": {
			v:      Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
			expect: String("2009-11-10T23:00:00Z"),
		},
		"duration": {
			v:      Duration(time.Second),
			expect: String("1s"),
		},
		"not UTF-8 encoded string bytes": {
			v:           Bytes([]byte("\xF4\x90\x80\x80")), // U+10FFFF+1; out of range
			expectError: "can't convert bytes to string: invalid UTF-8 encoded characters in bytes",
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
			got, err := stringType.Convert(test.v)
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

func TestString_Type(t *testing.T) {
	v := String("")
	if got, expect := v.Type().Name(), stringType.Name(); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestString_GoValue(t *testing.T) {
	v := String("test")
	if got, expect := v.GoValue(), "test"; got != expect {
		t.Errorf("expect %v but got %v", expect, got)
	}
}

func TestString_Equal(t *testing.T) {
	tests := map[string]struct {
		x           String
		y           Value
		expect      interface{}
		expectError string
	}{
		`"foo" == "foo"`: {
			x:      String("foo"),
			y:      String("foo"),
			expect: Bool(true),
		},
		`"foo" == "bar"`: {
			x:      String("foo"),
			y:      String("bar"),
			expect: Bool(false),
		},
		`"foo" == true`: {
			x:           String("foo"),
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

func TestString_Compare(t *testing.T) {
	tests := map[string]struct {
		x           String
		y           Value
		expect      interface{}
		expectError string
	}{
		`"foo" == "foo"`: {
			x:      String("foo"),
			y:      String("foo"),
			expect: Int(0),
		},
		`"foo" > "bar"`: {
			x:      String("foo"),
			y:      String("bar"),
			expect: Int(1),
		},
		`"bar" < "foo"`: {
			x:      String("bar"),
			y:      String("foo"),
			expect: Int(-1),
		},
		"nil is not string": {
			x:           String("foo"),
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

func TestString_Add(t *testing.T) {
	tests := map[string]struct {
		x           String
		y           Value
		expect      interface{}
		expectError string
	}{
		`foo + bar`: {
			x:      String("foo"),
			y:      String("bar"),
			expect: String("foobar"),
		},
		"nil is not string": {
			x:           String("foo"),
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

func TestString_Size(t *testing.T) {
	tests := map[string]struct {
		v           String
		expect      Value
		expectError string
	}{
		"unibyte": {
			v:      String("test"),
			expect: Int(4),
		},
		"multibyte": {
			v:      String("テスト"),
			expect: Int(3),
		},
	}
	for name, test := range tests {
		test := test
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
