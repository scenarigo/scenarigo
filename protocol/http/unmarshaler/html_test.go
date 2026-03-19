package unmarshaler

import (
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

func TestHTMLUnUnmarshaler_MediaType(t *testing.T) {
	var um htmlUnmarshaler
	if got, expect := um.MediaType(), "text/html"; got != expect {
		t.Fatalf("expect %q but got %q", expect, got)
	}
}

func TestHTMLUnUnmarshaler_Unmarshal(t *testing.T) {
	r := utf8.RuneError
	tests := map[string]struct {
		data   []byte
		expect any
	}{
		"utf8 string": {
			data: []byte(`<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>hello</title>
  </head>
  <body>world</body>
</html>`),
			expect: `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>hello</title>
  </head>
  <body>world</body>
</html>`,
		},
		"not utf8 string": {
			data:   append([]byte{}, byte(r)),
			expect: append([]byte{}, byte(r)),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var um htmlUnmarshaler
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
