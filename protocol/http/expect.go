package http

import (
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/assertutil"
)

// Expect represents expected response values.
type Expect struct {
	Code   string        `yaml:"code,omitempty"`
	Header yaml.MapSlice `yaml:"header,omitempty"`
	Body   interface{}   `yaml:"body,omitempty"`
}

// Build implements protocol.AssertionBuilder interface.
func (e *Expect) Build(ctx *context.Context) (assert.Assertion, error) {
	expectCode := "200"
	if e.Code != "" {
		expectCode = e.Code
	}

	codeAssertion, err := assert.Build(ctx.RequestContext(), expectCode, assert.FromTemplate(ctx))
	if err != nil {
		return nil, errors.WrapPathf(err, "code", "invalid expect status code")
	}

	headerAssertion, err := assertutil.BuildHeaderAssertion(ctx, e.Header)
	if err != nil {
		return nil, errors.WrapPathf(err, "header", "invalid expect header")
	}

	assertion, err := assert.Build(ctx.RequestContext(), e.Body, assert.FromTemplate(ctx))
	if err != nil {
		return nil, errors.WrapPathf(err, "body", "invalid expect response body")
	}

	return assert.AssertionFunc(func(v interface{}) error {
		res, ok := v.(response)
		if !ok {
			return errors.Errorf("expected response but got %T", v)
		}
		if err := assertCode(codeAssertion, res.Status); err != nil {
			return errors.WithPath(err, "code")
		}
		if err := headerAssertion.Assert(res.Header); err != nil {
			return errors.WithPath(err, "header")
		}
		if err := assertion.Assert(res.Body); err != nil {
			return errors.WithPath(err, "body")
		}
		return nil
	}), nil
}

func assertCode(assertion assert.Assertion, status string) error {
	strs := strings.SplitN(status, " ", 2)
	if len(strs) != 2 {
		return errors.Errorf(`unexpected response status string: "%s"`, status)
	}
	if err := assertion.Assert(strs[0]); err == nil {
		return nil
	}
	err := assertion.Assert(strs[1])
	if err == nil {
		return nil
	}
	return err
}
