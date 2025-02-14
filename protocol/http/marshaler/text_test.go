package marshaler

import "testing"

func TestText_Marshal(t *testing.T) {
	m := textMarshaler{}
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			v      interface{}
			expect string
		}{
			"[]byte": {
				v:      []byte("hello"),
				expect: "hello",
			},
			"string": {
				v:      "hello",
				expect: "hello",
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				b, err := m.Marshal(test.v)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if got := string(b); got != test.expect {
					t.Errorf("expect %q but got %q", test.expect, got)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			v      interface{}
			expect string
		}{
			"nil": {
				v:      nil,
				expect: "invalid value",
			},
			"struct": {
				v:      t,
				expect: "expected string but got testing.T",
			},
		}
		for name, test := range tests {
			m := textMarshaler{}
			t.Run(name, func(t *testing.T) {
				_, err := m.Marshal(test.v)
				if err == nil {
					t.Fatal("no error")
				}
				if got := err.Error(); got != test.expect {
					t.Errorf("expect %q but got %q", test.expect, got)
				}
			})
		}
	})
}
