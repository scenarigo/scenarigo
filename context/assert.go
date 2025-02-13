package context

import (
	"context"

	"github.com/pkg/errors"

	"github.com/scenarigo/scenarigo/assert"
)

type assertions struct {
	ctx context.Context
}

// ExtractByKey implements query.KeyExtractor interface.
func (a *assertions) ExtractByKey(key string) (interface{}, bool) {
	switch key {
	case "and":
		return listArgsLeftArrowFunc(buildArgs(a.ctx, assert.And)), true
	case "or":
		return listArgsLeftArrowFunc(buildArgs(a.ctx, assert.Or)), true
	case "contains":
		return &leftArrowFunc{
			ctx: a.ctx,
			f:   buildArg(a.ctx, assert.Contains),
		}, true
	case "notContains":
		return &leftArrowFunc{
			ctx: a.ctx,
			f:   buildArg(a.ctx, assert.NotContains),
		}, true
	case "notZero":
		return assert.NotZero(), true
	case "regexp":
		return assert.Regexp, true
	case "greaterThan":
		return assert.Greater, true
	case "greaterThanOrEqual":
		return assert.GreaterOrEqual, true
	case "lessThan":
		return assert.Less, true
	case "lessThanOrEqual":
		return assert.LessOrEqual, true
	case "length":
		return assert.Length, true
	}
	return nil, false
}

func buildArg(ctx context.Context, base func(assert.Assertion) assert.Assertion) func(interface{}) assert.Assertion {
	return func(arg interface{}) assert.Assertion {
		assertion, ok := arg.(assert.Assertion)
		if !ok {
			assertion = assert.MustBuild(ctx, arg)
		}
		return base(assertion)
	}
}

type leftArrowFunc struct {
	ctx context.Context
	f   func(interface{}) assert.Assertion
}

func (laf *leftArrowFunc) Call(v any) assert.Assertion {
	return laf.f(v)
}

func (laf *leftArrowFunc) Exec(arg interface{}) (interface{}, error) {
	assertion, ok := arg.(assert.Assertion)
	if !ok {
		return nil, errors.New("argument must be a assert.Assertion")
	}
	return laf.f(assertion), nil
}

func (laf *leftArrowFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var i interface{}
	if err := unmarshal(&i); err != nil {
		return nil, err
	}
	return assert.Build(laf.ctx, i)
}

func buildArgs(ctx context.Context, base func(...assert.Assertion) assert.Assertion) func(...interface{}) assert.Assertion {
	return func(args ...interface{}) assert.Assertion {
		var assertions []assert.Assertion
		for _, arg := range args {
			arg := arg
			assertion, ok := arg.(assert.Assertion)
			if !ok {
				assertion = assert.MustBuild(ctx, arg)
			}
			assertions = append(assertions, assertion)
		}
		return base(assertions...)
	}
}

type listArgsLeftArrowFunc func(args ...interface{}) assert.Assertion

func (f listArgsLeftArrowFunc) Exec(arg interface{}) (interface{}, error) {
	assertions, ok := arg.([]interface{})
	if !ok {
		return nil, errors.New("argument must be a slice of interface{}")
	}
	return f(assertions...), nil
}

func (listArgsLeftArrowFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var args []interface{}
	if err := unmarshal(&args); err != nil {
		return nil, err
	}
	return args, nil
}
