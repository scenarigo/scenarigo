package val

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/internal/testutil"
)

func TestDurationType_Name(t *testing.T) {
	v := durationType
	if got, expect := v.Name(), "duration"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestDurationType_NewValue(t *testing.T) {
	tests := map[string]struct {
		v           any
		expect      Value
		expectError string
	}{
		"duration": {
			v:      time.Second,
			expect: Duration(time.Second),
		},
		"string": {
			v:           "1s",
			expectError: ErrUnsupportedType.Error(),
		},
		"nil": {
			v:           nil,
			expectError: ErrUnsupportedType.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := durationType.NewValue(test.v)
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

func TestDurationType_Convert(t *testing.T) {
	tests := map[string]struct {
		v           Value
		expect      Value
		expectError string
	}{
		"duration": {
			v:      Duration(time.Second),
			expect: Duration(time.Second),
		},
		"any[*time.Duration]": {
			v:      Any{testutil.ToPtr(time.Second)},
			expect: Duration(time.Second),
		},
		"int": {
			v:      Int(1),
			expect: Duration(time.Nanosecond),
		},
		"string": {
			v:      String("1s"),
			expect: Duration(time.Second),
		},
		"invalid string": {
			v:           String("1"),
			expectError: `can't convert string to duration: time: missing unit in duration "1"`,
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
			got, err := durationType.Convert(test.v)
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

func TestDuration_Type(t *testing.T) {
	v := Duration(0)
	if got, expect := v.Type().Name(), durationType.Name(); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestDuration_GoValue(t *testing.T) {
	v := Duration(time.Second)
	got, expect := v.GoValue(), time.Second
	if diff := cmp.Diff(expect, got); diff != "" {
		t.Errorf("diff: (-want +got)\n%s", diff)
	}
}

func TestDuration_Neg(t *testing.T) {
	tests := map[string]struct {
		x           Duration
		expect      any
		expectError string
	}{
		"1s": {
			x:      Duration(time.Second),
			expect: Duration(-time.Second),
		},
		"max int": {
			x:      Duration(math.MaxInt64),
			expect: Duration(-math.MaxInt64),
		},
		"-1s": {
			x:      Duration(-time.Second),
			expect: Duration(time.Second),
		},
		"min int": {
			x:           Duration(math.MinInt64),
			expectError: fmt.Sprintf("-(%d) overflows duration", math.MinInt64),
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

func TestDuration_Equal(t *testing.T) {
	tests := map[string]struct {
		x           Duration
		y           Value
		expect      any
		expectError string
	}{
		"1s == 1s": {
			x:      Duration(time.Second),
			y:      Duration(time.Second),
			expect: Bool(true),
		},
		"1s == 2s": {
			x:      Duration(time.Second),
			y:      Duration(2 * time.Second),
			expect: Bool(false),
		},
		"1s == nil": {
			x:           Duration(time.Second),
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

func TestDuration_Compare(t *testing.T) {
	tests := map[string]struct {
		x           Duration
		y           Value
		expect      any
		expectError string
	}{
		"1s == 1s": {
			x:      Duration(time.Second),
			y:      Duration(time.Second),
			expect: Int(0),
		},
		"2s > 1s": {
			x:      Duration(2 * time.Second),
			y:      Duration(time.Second),
			expect: Int(1),
		},
		"1s < 2s": {
			x:      Duration(time.Second),
			y:      Duration(2 * time.Second),
			expect: Int(-1),
		},
		"nil is not duration": {
			x:           Duration(time.Second),
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

func TestDuration_Add(t *testing.T) {
	x := time.Now()
	y := x.Add(time.Hour)
	tests := map[string]struct {
		x           Duration
		y           Value
		expect      Value
		expectError string
	}{
		"1s + 1s": {
			x:      Duration(time.Second),
			y:      Duration(time.Second),
			expect: Duration(2 * time.Second),
		},
		"now + 1h": {
			x:      Duration(time.Hour),
			y:      Time(x),
			expect: Time(y),
		},
		"nil is not duration": {
			x:           Duration(time.Second),
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
				return
			}
			if diff := cmp.Diff(test.expect.GoValue(), got.GoValue()); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestDuration_Sub(t *testing.T) {
	tests := map[string]struct {
		x           Duration
		y           Value
		expect      any
		expectError string
	}{
		"1s - 1s": {
			x:      Duration(time.Second),
			y:      Duration(time.Second),
			expect: Duration(0),
		},
		"0s - 1s": {
			x:      Duration(0),
			y:      Duration(time.Second),
			expect: Duration(-time.Second),
		},
		"nil is not duration/time": {
			x:           Duration(time.Second),
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
