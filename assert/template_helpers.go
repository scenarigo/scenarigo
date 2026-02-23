package assert

import (
	"context"

	"github.com/pkg/errors"

	sccontext "github.com/scenarigo/scenarigo/context"
)

func init() {
	sccontext.RegisterNewAssertionsFunc(func(ctx context.Context) any {
		return &assertions{ctx: ctx}
	})
}

// assertions provides assertion helper functions for template context.
// It implements the query.KeyExtractor interface.
type assertions struct {
	ctx context.Context
}

// ExtractByKey implements query.KeyExtractor interface.
func (a *assertions) ExtractByKey(key string) (any, bool) {
	switch key {
	case "and":
		return listArgsLeftArrowFunc(buildAssertionArgs(a.ctx, And)), true
	case "or":
		return listArgsLeftArrowFunc(buildAssertionArgs(a.ctx, Or)), true
	case "contains":
		return &leftArrowFunc{
			ctx: a.ctx,
			f:   buildAssertionArg(a.ctx, Contains),
		}, true
	case "notContains":
		return &leftArrowFunc{
			ctx: a.ctx,
			f:   buildAssertionArg(a.ctx, NotContains),
		}, true
	case "any":
		return Nop(), true
	case "notZero":
		return NotZero(), true
	case "regexp":
		return Regexp, true
	case "greaterThan":
		return Greater, true
	case "greaterThanOrEqual":
		return GreaterOrEqual, true
	case "lessThan":
		return Less, true
	case "lessThanOrEqual":
		return LessOrEqual, true
	case "length":
		return Length, true
	}
	return nil, false
}

func buildAssertionArg(ctx context.Context, base func(Assertion) Assertion) func(any) Assertion {
	return func(arg any) Assertion {
		assertion, ok := arg.(Assertion)
		if !ok {
			assertion = MustBuild(ctx, arg)
		}
		return base(assertion)
	}
}

type leftArrowFunc struct {
	ctx context.Context
	f   func(any) Assertion
}

func (laf *leftArrowFunc) Call(v any) Assertion {
	return laf.f(v)
}

func (laf *leftArrowFunc) Exec(arg any) (any, error) {
	assertion, ok := arg.(Assertion)
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
	return Build(laf.ctx, i)
}

func buildAssertionArgs(ctx context.Context, base func(...Assertion) Assertion) func(...any) Assertion {
	return func(args ...any) Assertion {
		var assertions []Assertion
		for _, arg := range args {
			assertion, ok := arg.(Assertion)
			if !ok {
				assertion = MustBuild(ctx, arg)
			}
			assertions = append(assertions, assertion)
		}
		return base(assertions...)
	}
}

type listArgsLeftArrowFunc func(args ...any) Assertion

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
