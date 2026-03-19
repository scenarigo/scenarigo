package schema

import (
	"testing"
	"time"
)

func TestDuration_MarshalYAML(t *testing.T) {
	d := Duration(time.Hour + time.Minute + time.Second)
	b, err := d.MarshalYAML()
	if err != nil {
		t.Fatalf("failed to marshal: %s", err)
	}
	if got, expect := string(b), "1h1m1s"; got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestDuration_UnmarshalYAML(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			in     string
			expect Duration
		}{
			"valid": {
				in:     "1s",
				expect: Duration(time.Second),
			},
			"quoted": {
				in:     "'1s'",
				expect: Duration(time.Second),
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				var d Duration
				if err := d.UnmarshalYAML([]byte(test.in)); err != nil {
					t.Fatalf("failed to unmarshal: %s", err)
				}
				if got, expect := d, test.expect; got != expect {
					t.Errorf("expect %q but got %q", expect, got)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			in     string
			expect string
		}{
			"not duration string": {
				in:     "invalid",
				expect: `time: invalid duration "invalid"`,
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				var d Duration
				err := d.UnmarshalYAML([]byte(test.in))
				if err == nil {
					t.Fatal("no error")
				}
				if got, expect := err.Error(), test.expect; got != expect {
					t.Errorf("expect %q but got %q", expect, got)
				}
			})
		}
	})
}
