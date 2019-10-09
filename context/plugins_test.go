package context

import (
	"io"
	"plugin"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPlug_ExtactByKey(t *testing.T) {
	p, err := plugin.Open("../testdata/gen/plugins/simple.so")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	ptrSym, err := p.Lookup("Pointer")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tests := map[string]struct {
		key    string
		expect interface{}
	}{
		"string": {
			key:    "String",
			expect: "string",
		},
		"pointer": {
			key:    "Pointer",
			expect: reflect.ValueOf(ptrSym).Elem().Interface(),
		},
		"interface": {
			key:    "Interface",
			expect: io.Reader(nil),
		},
		"function": {
			key:    "Function",
			expect: "function",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			got, ok := (*plug)(p).ExtractByKey(test.key)
			if !ok {
				t.Fatal("not found")
			}
			if f, ok := got.(func() string); ok {
				got = f()
			}
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
		})
	}
}
