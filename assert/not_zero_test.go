package assert

import (
	"fmt"
	"testing"

	"github.com/zoncoen/query-go"
)

func TestNotZero(t *testing.T) {
	type myString string
	type myStruct struct {
		name string
	}
	tests := []struct {
		ok interface{}
		ng interface{}
	}{
		{
			ok: 1,
			ng: 0,
		},
		{
			ok: "test",
			ng: "",
		},
		{
			ok: myString("test"),
			ng: myString(""),
		},
		{
			ok: myStruct{name: "test"},
			ng: myStruct{},
		},
		{
			ok: &myStruct{},
			ng: nil,
		},
	}
	for i, test := range tests {
		test := test
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			assertion := NotZero(query.New())
			if err := assertion.Assert(test.ok); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if err := assertion.Assert(test.ng); err == nil {
				t.Errorf("expected error but no error")
			}
		})
	}
}
