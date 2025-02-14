// Package assert provides value assertions.
package assert

import (
	"context"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/zoncoen/query-go"

	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/template"
)

// Assertion implements value assertion.
type Assertion interface {
	Assert(v interface{}) error
}

// AssertionFunc is an adaptor to allow the use of ordinary functions as assertions.
type AssertionFunc func(v interface{}) error

// Assert asserts the v.
func (f AssertionFunc) Assert(v interface{}) error {
	return f(v)
}

type buildOpt struct {
	tmplData any
	eqs      []Equaler
}

// BuildOpt represents an option for Build().
type BuildOpt func(*buildOpt)

// FromTemplate is a build option that executes templates before building assertions.
func FromTemplate(data any) BuildOpt {
	return func(opt *buildOpt) {
		opt.tmplData = data
	}
}

// WithEqualers is a build option that enables custom equalers.
func WithEqualers(eqs ...Equaler) BuildOpt {
	return func(opt *buildOpt) {
		opt.eqs = append(opt.eqs, eqs...)
	}
}

// Build builds an assertion from Go value.
// If the Assert method of built assertion isn't called, the context value should be canceled to avoid a goroutine leak.
func Build(ctx context.Context, expect any, fs ...BuildOpt) (Assertion, error) {
	var opt buildOpt
	for _, f := range fs {
		f(&opt)
	}
	var assertions []Assertion
	if expect != nil {
		var err error
		assertions, err = build(ctx, queryutil.New(), expect, &opt)
		if err != nil {
			return nil, fmt.Errorf("failed to build assertion: %w", err)
		}
	}
	return AssertionFunc(func(v interface{}) error {
		errs := []error{}
		for _, assertion := range assertions {
			if err := assertion.Assert(v); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			if len(errs) == 1 {
				return errs[0]
			}
			return errors.Errors(errs...)
		}
		return nil
	}), nil
}

// MustBuild builds an assertion from Go value.
// If the Assert method of built assertion isn't called, the context value should be canceled to avoid a goroutine leak.
// If it fails to build, creates an assertion function that returns the build error.
func MustBuild(ctx context.Context, expect any, fs ...BuildOpt) Assertion {
	assertion, err := Build(ctx, expect, fs...)
	if err != nil {
		return AssertionFunc(func(_ any) error {
			return err
		})
	}
	return assertion
}

func build(ctx context.Context, q *query.Query, expect any, opt *buildOpt) ([]Assertion, error) {
	var assertions []Assertion
	switch v := expect.(type) {
	case yaml.MapSlice:
		for _, item := range v {
			k, err := template.Execute(ctx, item.Key, opt.tmplData)
			if err != nil {
				return nil, err
			}
			if f, ok := k.(*template.FuncCall); ok {
				if len(v) != 1 {
					return nil, errors.New("invalid left arrow function call")
				}
				path := fmt.Sprintf(".'%s'", item.Key)
				res, err := f.Do(ctx, item.Value, opt.tmplData)
				if err != nil {
					return nil, errors.WithPath(errors.Wrap(err, "failed to execute left arrow function"), path)
				}
				as, err := build(ctx, q, res, opt)
				if err != nil {
					return nil, errors.WithPath(err, path)
				}
				for i, a := range as {
					as[i] = AssertionFunc(func(v any) error {
						if err := a.Assert(v); err != nil {
							return errors.ReplacePath(err, q.String(), q.String()+path)
						}
						return nil
					})
				}
				assertions = append(assertions, as...)
			} else {
				as, err := build(ctx, q.Key(fmt.Sprintf("%s", k)), item.Value, opt)
				if err != nil {
					return nil, errors.WithPath(err, fmt.Sprint(item.Key))
				}
				assertions = append(assertions, as...)
			}
		}
	case []interface{}:
		for i, elm := range v {
			as, err := build(ctx, q.Index(i), elm, opt)
			if err != nil {
				return nil, errors.WithPath(err, fmt.Sprintf("[%d]", i))
			}
			assertions = append(assertions, as...)
		}
	default:
		switch v := expect.(type) {
		case string:
			return buildAssertion(ctx, q, v, opt)
		case Assertion:
			assertions = append(assertions, AssertionFunc(func(val interface{}) error {
				got, err := q.Extract(val)
				if err != nil {
					return err
				}
				if err := v.Assert(got); err != nil {
					return errors.WithQuery(err, q)
				}
				return nil
			}))
		case func(*query.Query) Assertion:
			assertions = append(assertions, v(q))
		case template.Lazy:
			assertions = append(assertions, lazyAssertion(q, v))
		default:
			as, err := build(ctx, q, Equal(v, opt.eqs...), opt)
			if err != nil {
				return nil, err
			}
			assertions = append(assertions, as...)
		}
	}
	return assertions, nil
}

func buildAssertion(ctx context.Context, q *query.Query, expect any, opt *buildOpt) ([]Assertion, error) {
	v, err := template.Execute(ctx, expect, opt.tmplData)
	if err != nil {
		return nil, err
	}
	if s, ok := v.(string); ok {
		v = Equal(s, opt.eqs...)
	}
	return build(ctx, q, v, opt)
}

func lazyAssertion(q *query.Query, f template.Lazy) Assertion {
	return AssertionFunc(func(val interface{}) error {
		v, err := q.Extract(val)
		if err != nil {
			return err
		}
		res, err := f(v)
		if err != nil {
			return errors.WithQuery(err, q)
		}
		if pass, err := convert(res, false); err == nil {
			if pass {
				return nil
			}
			return errors.ErrorQueryf(q, "assertion error")
		}
		return errors.ErrorQueryf(q, "assertion result must be a boolean value but got %T", res)
	})
}
