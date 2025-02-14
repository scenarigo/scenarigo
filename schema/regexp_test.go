package schema

import (
	"fmt"
	"testing"
)

func TestRegexp_MarshalYAML(t *testing.T) {
	s := ".+"
	r := Regexp{str: s}
	b, err := r.MarshalYAML()
	if err != nil {
		t.Fatalf("failed to marshal: %s", err)
	}
	if got, expect := string(b), fmt.Sprintln(s); got != expect {
		t.Errorf("expect %q but got %q", expect, got)
	}
}

func TestRegexp_UnmarshalYAML(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			in     string
			expect *Regexp
			ok     string
			ng     string
		}{
			"valid": {
				in: "test",
				expect: &Regexp{
					str: "test",
				},
				ok: "test",
				ng: "aaa",
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				var r Regexp
				if err := r.UnmarshalYAML([]byte(test.in)); err != nil {
					t.Fatalf("failed to unmarshal: %s", err)
				}
				if got, expect := r.str, test.expect.str; got != expect {
					t.Errorf("expect %q but got %q", expect, got)
				}
				if !r.MatchString(test.ok) {
					t.Errorf("not match %q", test.ok)
				}
				if r.MatchString(test.ng) {
					t.Errorf("match %q", test.ng)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			in     string
			expect string
		}{
			"invalid pattern": {
				in:     "(",
				expect: "error parsing regexp: missing closing ): `(`",
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				var r Regexp
				err := r.UnmarshalYAML([]byte(test.in))
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
