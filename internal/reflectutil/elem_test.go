package reflectutil

import (
	"reflect"
	"testing"
)

func TestElem(t *testing.T) {
	s := "test"
	p := &s
	var i any
	tests := map[string]struct {
		v      any
		expect reflect.Kind
	}{
		"string": {
			v:      s,
			expect: reflect.String,
		},
		"*string": {
			v:      p,
			expect: reflect.String,
		},
		"**string": {
			v:      &p,
			expect: reflect.String,
		},
		"nil interface{}": {
			v:      i,
			expect: reflect.Invalid,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := Elem(reflect.ValueOf(test.v))
			if k := got.Kind(); k != test.expect {
				t.Fatalf("expect %s but got %s", test.expect.String(), k.String())
			}
		})
	}
}
