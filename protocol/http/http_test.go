package http

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/protocol"
)

func TestHTTP(t *testing.T) {
	Register()
	p := protocol.Get("http")
	if p == nil {
		t.Fatal("http protocol not found")
	}
	if err := p.UnmarshalOption([]byte("")); err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_UnmarshalRequest(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tests := map[string]struct {
			bytes  []byte
			expect *Request
		}{
			"default": {
				bytes:  nil,
				expect: &Request{},
			},
			"method": {
				bytes: []byte(`method: GET`),
				expect: &Request{
					Method: "GET",
				},
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				p := &HTTP{}
				invoker, err := p.UnmarshalRequest(test.bytes)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.expect, invoker); diff != "" {
					t.Errorf("request differs (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("ng", func(t *testing.T) {
		tests := map[string]struct {
			bytes []byte
		}{
			"unknown field": {
				bytes: []byte(`a: b`),
			},
			"duplicated field": {
				bytes: []byte(`
method: GET
method: GET`),
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				p := &HTTP{}
				_, err := p.UnmarshalRequest(test.bytes)
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
			})
		}
	})
}

func TestHTTP_UnmarshalExpect(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tests := map[string]struct {
			bytes  []byte
			expect *Expect
		}{
			"default": {
				bytes:  nil,
				expect: &Expect{},
			},
			"code": {
				bytes: []byte(`code: 404`),
				expect: &Expect{
					Code: "404",
				},
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				p := &HTTP{}
				builder, err := p.UnmarshalExpect(test.bytes)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.expect, builder); diff != "" {
					t.Errorf("request differs (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("ng", func(t *testing.T) {
		tests := map[string]struct {
			bytes []byte
		}{
			"unknown field": {
				bytes: []byte(`a: b`),
			},
			"duplicated field": {
				bytes: []byte("code: 404\ncode: 404"),
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				p := &HTTP{}
				_, err := p.UnmarshalExpect(test.bytes)
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
			})
		}
	})
}
