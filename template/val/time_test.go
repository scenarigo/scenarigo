package val

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/scenarigo/scenarigo/internal/testutil"
)

func TestTimeType_Name(t *testing.T) {
	v := timeType
	if got, expect := v.Name(), "time"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestTimeType_NewValue(t *testing.T) {
	now := time.Now()
	tests := map[string]struct {
		v           any
		expect      Value
		expectError string
	}{
		"time": {
			v:      now,
			expect: Time(now),
		},
		"bool": {
			v:           true,
			expectError: ErrUnsupportedType.Error(),
		},
		"int": {
			v:           1,
			expectError: ErrUnsupportedType.Error(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := timeType.NewValue(test.v)
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
			if diff := cmp.Diff(test.expect, got, cmp.AllowUnexported(Time{}, time.Location{})); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestTimeType_Convert(t *testing.T) {
	tm := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		v           Value
		expect      Value
		expectError string
	}{
		"time": {
			v:      Time(tm),
			expect: Time(tm),
		},
		"any[*time]": {
			v:      Any{testutil.ToPtr(tm)},
			expect: Time(tm),
		},
		"string": {
			v:      String(tm.Format(time.RFC3339)),
			expect: Time(tm),
		},
		"invalid string": {
			v:           String("test"),
			expectError: `can't convert string to time: parsing time "test" as "2006-01-02T15:04:05Z07:00": cannot parse "test" as "2006"`,
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
			got, err := timeType.Convert(test.v)
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
			if diff := cmp.Diff(test.expect, got, cmpopts.IgnoreUnexported(Time{}, time.Location{})); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestTime_Type(t *testing.T) {
	v := Time{}
	if got, expect := v.Type().Name(), timeType.Name(); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestTime_GoValue(t *testing.T) {
	now := time.Now()
	v := Time(now)
	if got, expect := v.GoValue(), now; got != expect {
		t.Errorf("expect %v but got %v", expect, got)
	}
}

func TestTime_Equal(t *testing.T) {
	x := time.Now()
	y := x.Add(time.Hour)
	tests := map[string]struct {
		x           Time
		y           Value
		expect      interface{}
		expectError string
	}{
		"x == x": {
			x:      Time(x),
			y:      Time(x),
			expect: Bool(true),
		},
		"x == y": {
			x:      Time(x),
			y:      Time(y),
			expect: Bool(false),
		},
		"x == true": {
			x:           Time(x),
			y:           Bool(true),
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

func TestTime_Compare(t *testing.T) {
	x := time.Now()
	y := x.Add(time.Hour)
	tests := map[string]struct {
		x           Time
		y           Value
		expect      interface{}
		expectError string
	}{
		`x == y`: {
			x:      Time(x),
			y:      Time(x),
			expect: Int(0),
		},
		`x < y`: {
			x:      Time(x),
			y:      Time(y),
			expect: Int(-1),
		},
		`y > x`: {
			x:      Time(y),
			y:      Time(x),
			expect: Int(1),
		},
		"nil is not time": {
			x:           Time(x),
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

func TestTime_Add(t *testing.T) {
	x := time.Now()
	y := x.Add(time.Hour)
	z := x.Add(-time.Hour)
	tests := map[string]struct {
		x           Time
		y           Value
		expect      Value
		expectError string
	}{
		`x + 1h`: {
			x:      Time(x),
			y:      Duration(time.Hour),
			expect: Time(y),
		},
		`x + -1h`: {
			x:      Time(x),
			y:      Duration(-time.Hour),
			expect: Time(z),
		},
		"nil is not time": {
			x:           Time(x),
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

func TestTime_Sub(t *testing.T) {
	x := time.Now()
	y := x.Add(time.Hour)
	z := x.Add(-time.Hour)
	tests := map[string]struct {
		x           Time
		y           Value
		expect      Value
		expectError string
	}{
		`x - y`: {
			x:      Time(x),
			y:      Time(y),
			expect: Duration(-time.Hour),
		},
		`x - 1h`: {
			x:      Time(x),
			y:      Duration(time.Hour),
			expect: Time(z),
		},
		`x - -1h`: {
			x:      Time(x),
			y:      Duration(-time.Hour),
			expect: Time(y),
		},
		"nil is not time/duration": {
			x:           Time(x),
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
				return
			}
			if diff := cmp.Diff(test.expect.GoValue(), got.GoValue()); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}
