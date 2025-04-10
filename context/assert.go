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
func (a *assertions) ExtractByKey(key string) (any, bool) {
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

func buildArg(ctx context.Context, base func(assert.Assertion) assert.Assertion) func(any) assert.Assertion {
	return func(arg any) assert.Assertion {
		assertion, ok := arg.(assert.Assertion)
		if !ok {
			assertion = assert.MustBuild(ctx, arg)
		}
		return base(assertion)
	}
}

type leftArrowFunc struct {
	ctx context.Context
	f   func(any) assert.Assertion
}

func (laf *leftArrowFunc) Call(v any) assert.Assertion {
	return laf.f(v)
}

func (laf *leftArrowFunc) Exec(arg any) (any, error) {
	assertion, ok := arg.(assert.Assertion)
	if !ok {
		return nil, errors.New("argument must be a assert.Assertion")
	}
	return laf.f(assertion), nil
}

func (laf *leftArrowFunc) UnmarshalArg(unmarshal func(any) error) (any, error) {
	var i any
	if err := unmarshal(&i); err != nil {
		return nil, err
	}
	return assert.Build(laf.ctx, i)
}

func buildArgs(ctx context.Context, base func(...assert.Assertion) assert.Assertion) func(...any) assert.Assertion {
	return func(args ...any) assert.Assertion {
		var assertions []assert.Assertion
		for _, arg := range args {
			assertion, ok := arg.(assert.Assertion)
			if !ok {
				assertion = assert.MustBuild(ctx, arg)
			}
			assertions = append(assertions, assertion)
		}
		return base(assertions...)
	}
}

type listArgsLeftArrowFunc func(args ...any) assert.Assertion

func (f listArgsLeftArrowFunc) Exec(arg any) (any, error) {
	assertions, ok := arg.([]any)
	if !ok {
		return nil, errors.New("argument must be a slice of interface{}")
	}
	return f(assertions...), nil
}

func (listArgsLeftArrowFunc) UnmarshalArg(unmarshal func(any) error) (any, error) {
	var args []any
	if err := unmarshal(&args); err != nil {
		return nil, err
	}
	return args, nil
}
