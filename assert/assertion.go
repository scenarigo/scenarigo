// Package assert provides value assertions.
package assert

import (
	"errors"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/zoncoen/query-go"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/query/extractor"
)

// Assertion implements value assertion.
type Assertion interface {
	Assert(ctx *context.Context, v interface{}) error
}

// AssertionFunc is an adaptor to allow the use of ordinary functions as assertions.
type AssertionFunc func(ctx *context.Context, v interface{}) error

// Assert asserts the v.
func (f AssertionFunc) Assert(ctx *context.Context, v interface{}) error {
	return f(ctx, v)
}

func assertFunc(q *query.Query, f func(*context.Context, interface{}) error) Assertion {
	return AssertionFunc(func(ctx *context.Context, v interface{}) error {
		got, err := q.Extract(v)
		if err != nil {
			query := q.String()
			if len(query) > 1 {
				ctx = ctx.AddChildPath(query[1:])
			}
			ctx.ReportYAML()
			return err
		}
		return f(ctx, got)
	})
}

var assertions = map[string]interface{}{
	"notZero":     NotZero,
	"contains":    leftArrowFunc(Contains),
	"notContains": leftArrowFunc(NotContains),
}

func init() {
	context.RegisterAssertions(assertions)
}

type leftArrowFunc func(assertion Assertion) func(*query.Query) Assertion

func (f leftArrowFunc) Exec(arg interface{}) (interface{}, error) {
	assertion, ok := arg.(Assertion)
	if !ok {
		return nil, errors.New("argument must be a assert.Assertion")
	}
	return f(assertion), nil
}

func (leftArrowFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var i interface{}
	if err := unmarshal(&i); err != nil {
		return nil, err
	}
	return Build(i), nil
}

// Build creates an assertion from Go value.
func Build(expect interface{}) Assertion {
	var assertions []Assertion
	if expect != nil {
		assertions = build(query.New(), expect)
	}
	return AssertionFunc(func(ctx *context.Context, v interface{}) error {
		var assertErr error
		for _, assertion := range assertions {
			assertion := assertion
			if err := assertion.Assert(ctx, v); err != nil {
				assertErr = AppendError(assertErr, err)
			}
		}
		return assertErr
	})
}

func build(q *query.Query, expect interface{}) []Assertion {
	var assertions []Assertion
	switch v := expect.(type) {
	case yaml.MapSlice:
		for _, item := range v {
			item := item
			key := fmt.Sprintf("%s", item.Key)
			assertions = append(assertions, build(q.Append(extractor.Key(key)), item.Value)...)
		}
	case []interface{}:
		for i, elm := range v {
			elm := elm
			assertions = append(assertions, build(q.Index(i), elm)...)
		}
	default:
		switch v := expect.(type) {
		case func(*query.Query) Assertion:
			assertions = append(assertions, v(q))
		default:
			assertions = append(assertions, Equal(q, v))
		}
	}
	return assertions
}
