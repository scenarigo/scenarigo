package assert

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/zoncoen/query-go"
	"github.com/zoncoen/scenarigo/context"
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
		{
			ok: json.Number("1"),
			ng: json.Number("0"),
		},
		{
			ok: json.Number("1.0"),
			ng: json.Number("0.0"),
		},
	}
	for i, test := range tests {
		test := test
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			ctx := context.FromT(t)
			assertion := NotZero(query.New())
			if err := assertion.Assert(ctx, test.ok); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if err := assertion.Assert(ctx, test.ng); err == nil {
				t.Errorf("expected error but no error")
			}
		})
	}
}
