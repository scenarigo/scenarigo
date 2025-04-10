package unmarshaler

import (
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

func TestTextUnUnmarshaler_MediaType(t *testing.T) {
	var um textUnmarshaler
	if got, expect := um.MediaType(), "text/plain"; got != expect {
		t.Fatalf("expect %q but got %q", expect, got)
	}
}

func TestTextUnUnmarshaler_Unmarshal(t *testing.T) {
	r := utf8.RuneError
	tests := map[string]struct {
		data   []byte
		expect any
	}{
		"utf8 string": {
			data:   []byte("test"),
			expect: "test",
		},
		"not utf8 string": {
			data:   append([]byte{}, byte(r)),
			expect: append([]byte{}, byte(r)),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var um textUnmarshaler
			var got any
			if err := um.Unmarshal(test.data, &got); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
