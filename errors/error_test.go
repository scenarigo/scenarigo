package errors

import (
	"fmt"
	"testing"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"
)

func validatePath(t *testing.T, e *PathError, path string) {
	t.Helper()
	if e.Path != fmt.Sprintf(".%s", path) {
		t.Fatalf("unexpected path: %s", e.Path)
	}
}

func TestNew(t *testing.T) {
	err := New("message")
	if _, ok := err.(Error); ok {
		t.Fatalf("expect github.com/pkg/errors instance. but %T", err)
	}
}

func TestErrorf(t *testing.T) {
	err := Errorf("%s", "message")
	if _, ok := err.(Error); ok {
		t.Fatalf("expect github.com/pkg/errors instance. but %T", err)
	}
}

func TestErrorPath(t *testing.T) {
	err := ErrorPath("path", "message")
	e, ok := err.(*PathError)
	if !ok {
		t.Fatalf("expect PathError instance. but %T", err)
	}
	validatePath(t, e, "path")
	if e.Err == nil {
		t.Fatal("err member must be not nil")
	}
}

func TestErrorPathf(t *testing.T) {
	err := ErrorPathf("path", "%s", "message")
	e, ok := err.(*PathError)
	if !ok {
		t.Fatalf("expect PathError instance. but %T", err)
	}
	validatePath(t, e, "path")
	if e.Err == nil {
		t.Fatal("err member must be not nil")
	}
}

func TestErrorQueryf(t *testing.T) {
	q, err := query.ParseString("path")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	errQuery := ErrorQueryf(q, "%s", "message")
	e, ok := errQuery.(*PathError)
	if !ok {
		t.Fatalf("expect PathError instance. but %T", errQuery)
	}
	validatePath(t, e, "path")
	if e.Err == nil {
		t.Fatal("err member must be not nil")
	}
}

func TestWrap(t *testing.T) {
	t.Run("wrap pkg/errors instance", func(t *testing.T) {
		err := Wrap(errors.New("message"), "message2")
		if err.Error() != "\n\nmessage2: message" {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
	})
	t.Run("wrap PathError instance", func(t *testing.T) {
		err := Wrap(ErrorPath("path", "message"), "message2")
		if err.Error() != "\n\nmessage2: message" {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
		e, ok := err.(*PathError)
		if !ok {
			t.Fatalf("expect PathError instance. but %T", err)
		}
		validatePath(t, e, "path")
	})
}

func TestWrapPath(t *testing.T) {
	t.Run("wrap pkg/errors instance", func(t *testing.T) {
		err := WrapPath(errors.New("message"), "path", "message2")
		if err.Error() != "\n\nmessage2: message" {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
	})
	t.Run("wrap PathError instance", func(t *testing.T) {
		err := WrapPath(ErrorPath("a", "message"), "b", "message2")
		if err.Error() != "\n\nmessage2: message" {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
		e, ok := err.(*PathError)
		if !ok {
			t.Fatalf("expect PathError instance. but %T", err)
		}
		validatePath(t, e, "b.a")
	})
}

func TestWrapf(t *testing.T) {
	t.Run("wrap pkg/errors instance", func(t *testing.T) {
		err := Wrapf(errors.New("message"), "%s", "message2")
		if err.Error() != "\n\nmessage2: message" {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
	})
	t.Run("wrap PathError instance", func(t *testing.T) {
		err := Wrapf(ErrorPath("path", "message"), "%s", "message2")
		if err.Error() != "\n\nmessage2: message" {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
		e, ok := err.(*PathError)
		if !ok {
			t.Fatalf("expect PathError instance. but %T", err)
		}
		validatePath(t, e, "path")
	})
}

func TestWrapPathf(t *testing.T) {
	t.Run("wrap pkg/errors instance", func(t *testing.T) {
		err := WrapPathf(errors.New("message"), "path", "%s", "message2")
		if err.Error() != "\n\nmessage2: message" {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
	})
	t.Run("wrap PathError instance", func(t *testing.T) {
		err := WrapPathf(ErrorPath("a", "message"), "b", "%s", "message2")
		if err.Error() != "\n\nmessage2: message" {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
		e, ok := err.(*PathError)
		if !ok {
			t.Fatalf("expect PathError instance. but %T", err)
		}
		validatePath(t, e, "b.a")
	})
}

func TestWithPath(t *testing.T) {
	t.Run("add path to pkg/errors instance", func(t *testing.T) {
		err := WithPath(errors.New("message"), "path")
		if err.Error() != "\n\nmessage" {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
	})
	t.Run("add path to PathError instance", func(t *testing.T) {
		err := WithPath(ErrorPath("a", "message"), "b")
		if err.Error() != "\n\nmessage" {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
		e, ok := err.(*PathError)
		if !ok {
			t.Fatalf("expect PathError instance. but %T", err)
		}
		validatePath(t, e, "b.a")
	})
}

func TestWithQuery(t *testing.T) {
	t.Run("add path by query to pkg/errors instance", func(t *testing.T) {
		q, err := query.ParseString("path")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		errQuery := WithQuery(errors.New("message"), q)
		if errQuery.Error() != "\n\nmessage" {
			t.Fatalf("unexpected error message: %s", errQuery.Error())
		}
		e, ok := errQuery.(*PathError)
		if !ok {
			t.Fatalf("expect PathError instance. but %T", errQuery)
		}
		validatePath(t, e, "path")
	})
	t.Run("add path by query to PathError instance", func(t *testing.T) {
		q, err := query.ParseString("b")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		errQuery := WithQuery(ErrorPath("a", "message"), q)
		if errQuery.Error() != "\n\nmessage" {
			t.Fatalf("unexpected error message: %s", errQuery.Error())
		}
		e, ok := errQuery.(*PathError)
		if !ok {
			t.Fatalf("expect PathError instance. but %T", errQuery)
		}
		validatePath(t, e, "b.a")
	})
}

func TestWithNodeAndColored(t *testing.T) {
	node := ast.String(token.String("a", "a", nil))
	err := WithNodeAndColored(ErrorPath("a", "message"), node, true)
	e, ok := err.(*PathError)
	if !ok {
		t.Fatalf("expect PathError instance. but %T", err)
	}
	if e.Node != node {
		t.Fatal("failed to set node")
	}
	if !e.EnabledColor {
		t.Fatal("failed to set colored")
	}
}

func TestMultiPathError(t *testing.T) {
	yml := `---
path:
  a: 1
  b: 2
  c: 3
`
	file, err := parser.ParseBytes([]byte(yml), 0)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if len(file.Docs) != 1 {
		t.Fatal("failed to parse YAML")
	}
	body := file.Docs[0].Body
	t.Run("one error", func(t *testing.T) {
		expected := `
1 error occurred:
   1 | ---
   2 | path:
>  3 |   a: 1
            ^
   4 |   b: 2
   5 |   c: 3
invalid a

`
		multiErr := WithNodeAndColored(
			Errors(ErrorPath("a", "invalid a")),
			body, false,
		)
		if _, ok := multiErr.(*MultiPathError); !ok {
			t.Fatalf("expect MultiPathError instance. but %T", err)
		}
		err := WithPath(multiErr, "path")
		if "\n"+err.Error() != expected {
			t.Fatal("\n" + err.Error())
		}
	})
	t.Run("with path", func(t *testing.T) {
		expected := `
3 errors occurred:
   1 | ---
   2 | path:
>  3 |   a: 1
            ^
   4 |   b: 2
   5 |   c: 3
invalid a

   1 | ---
   2 | path:
   3 |   a: 1
>  4 |   b: 2
            ^
   5 |   c: 3
invalid b
invalid c

`
		multiErr := WithNodeAndColored(
			Errors(
				ErrorPath("a", "invalid a"),
				ErrorPath("b", "invalid b"),
				errors.New("invalid c"),
			),
			body, false,
		)
		if _, ok := multiErr.(*MultiPathError); !ok {
			t.Fatalf("expect MultiPathError instance. but %T", err)
		}
		err := WithPath(multiErr, "path")
		if "\n"+err.Error() != expected {
			t.Fatal(err)
		}
	})
	t.Run("wrap path", func(t *testing.T) {
		expected := `
3 errors occurred:
   1 | ---
   2 | path:
>  3 |   a: 1
            ^
   4 |   b: 2
   5 |   c: 3
unexpected error: invalid a

   1 | ---
   2 | path:
   3 |   a: 1
>  4 |   b: 2
            ^
   5 |   c: 3
unexpected error: invalid b
unexpected error: invalid c

`
		multiErr := WithNodeAndColored(
			Errors(
				ErrorPath("a", "invalid a"),
				ErrorPath("b", "invalid b"),
				errors.New("invalid c"),
			),
			body, false,
		)
		if _, ok := multiErr.(*MultiPathError); !ok {
			t.Fatalf("expect MultiPathError instance. but %T", err)
		}
		err := WrapPath(multiErr, "path", "unexpected error")
		if "\n"+err.Error() != expected {
			t.Fatal(err)
		}
	})
}
