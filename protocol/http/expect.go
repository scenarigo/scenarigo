package http

import (
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/internal/maputil"
	"github.com/zoncoen/scenarigo/template"
)

// Expect represents expected response values.
type Expect struct {
	Code   string        `yaml:"code"`
	Header yaml.MapSlice `yaml:"header"`
	Body   interface{}   `yaml:"body"`
}

// Build implements protocol.AssertionBuilder interface.
func (e *Expect) Build(ctx *context.Context) (assert.Assertion, error) {
	expectBody, err := template.Execute(ctx.AddChildPath("body"), e.Body)
	if err != nil {
		return nil, errors.Errorf("invalid expect response: %s", err)
	}
	assertion := assert.Build(expectBody)

	return assert.AssertionFunc(func(ctx *context.Context, v interface{}) error {
		res, ok := v.(response)
		if !ok {
			return errors.Errorf("expected response but got %T", v)
		}
		if err := e.assertCode(ctx, res.status); err != nil {
			return err
		}
		if err := e.assertHeader(ctx, res.Header); err != nil {
			return err
		}
		if err := assertion.Assert(ctx, res.Body); err != nil {
			return err
		}
		return nil
	}), nil
}

func (e *Expect) assertHeader(ctx *context.Context, header map[string][]string) error {
	if len(e.Header) == 0 {
		return nil
	}
	headerMap, err := maputil.ConvertStringsMapSlice(e.Header)
	if err != nil {
		return err
	}
	if err := assert.Build(headerMap).Assert(ctx, header); err != nil {
		return err
	}
	return nil
}

func (e *Expect) assertCode(ctx *context.Context, status string) error {
	expectedCode := "200"
	if e.Code != "" {
		expectedCode = e.Code
	}
	strs := strings.SplitN(status, " ", 2)
	if len(strs) != 2 {
		return errors.Errorf(`unexpected response status string: "%s"`, status)
	}
	if got, expected := strs[0], expectedCode; got == expected {
		return nil
	}
	if got, expected := strs[1], expectedCode; got == expected {
		return nil
	}
	return errors.Errorf(`expected code is "%s" but got "%s"`, expectedCode, status)
}
