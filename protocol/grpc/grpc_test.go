package grpc

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/protocol"
)

func TestMain(m *testing.M) {
	p := &GRPC{}
	queryutil.AppendOptions(p.QueryOptions()...)
	os.Exit(m.Run())
}

func TestGRPC(t *testing.T) {
	Register()
	p := protocol.Get("grpc")
	if p == nil {
		t.Fatal("grpc protocol not found")
	}
	if err := p.UnmarshalOption([]byte("")); err != nil {
		t.Fatal(err)
	}
}

func TestGRPC_UnmarshalRequest(t *testing.T) {
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
				bytes: []byte(`method: Ping`),
				expect: &Request{
					Method: "Ping",
				},
			},
			"message": {
				bytes: []byte(`message: hello`),
				expect: &Request{
					Message: "hello",
				},
			},
			"body (check backward compatibility)": {
				bytes: []byte(`body: hello`),
				expect: &Request{
					Message: "hello",
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				p := &GRPC{}
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
method: Ping
method: Ping`),
			},
			"use body and message": {
				bytes: []byte(`
body: test
message: test`),
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				p := &GRPC{}
				_, err := p.UnmarshalRequest(test.bytes)
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
			})
		}
	})
}

func TestGRPC_UnmarshalExpect(t *testing.T) {
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
				bytes: []byte(`code: InvalidArgument`),
				expect: &Expect{
					Code: "InvalidArgument",
				},
			},
			"message": {
				bytes: []byte(`message: hello`),
				expect: &Expect{
					Message: "hello",
				},
			},
			"body (check backward compatibility)": {
				bytes: []byte(`body: hello`),
				expect: &Expect{
					Message: "hello",
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				p := &GRPC{}
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
				bytes: []byte("code: InvalidArgument\ncode: InvalidArgument"),
			},
			"use body and message": {
				bytes: []byte(`
body: test
message: test`),
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				p := &GRPC{}
				_, err := p.UnmarshalExpect(test.bytes)
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
			})
		}
	})
}

func TestGRPC_QueryOptions(t *testing.T) {
	p := &GRPC{}
	queryutil.AppendOptions(p.QueryOptions()...)
	got, err := queryutil.New().Key("b").Key("bar_value").Extract(&OneofMessage{
		Value: &OneofMessage_B_{
			B: &OneofMessage_B{
				BarValue: "yyy",
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to extract: %s", err)
	}
	if diff := cmp.Diff("yyy", got); diff != "" {
		t.Errorf("request differs (-want +got):\n%s", diff)
	}
}

type OneofMessage struct {
	Value isOneofMessage_Value `protobuf_oneof:"value"`
}

type isOneofMessage_Value interface {
	isOneofMessage_Value()
}

type OneofMessage_A_ struct {
	A *OneofMessage_A `protobuf:"bytes,1,opt,name=a,proto3,oneof"`
}

type OneofMessage_B_ struct {
	B *OneofMessage_B `protobuf:"bytes,2,opt,name=b,proto3,oneof"`
}

func (*OneofMessage_A_) isOneofMessage_Value() {}

func (*OneofMessage_B_) isOneofMessage_Value() {}

type OneofMessage_A struct {
	FooValue string `json:"foo_value,omitempty" protobuf:"bytes,1,opt,name=foo_value,json=fooValue,proto3"`
}

type OneofMessage_B struct {
	BarValue string `json:"bar_value,omitempty" protobuf:"bytes,1,opt,name=bar_value,json=barValue,proto3"`
}
