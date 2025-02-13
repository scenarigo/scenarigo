package assert

import (
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/queryutil"
)

// And returns a new assertion to ensure that value passes all assertions.
// If the assertions are empty, it returns an error.
func And(assertions ...Assertion) Assertion {
	return AssertionFunc(func(v interface{}) error {
		if len(assertions) == 0 {
			return errors.New("empty assertion list")
		}
		errs := []error{}
		for i, assertion := range assertions {
			assertion := assertion
			err := assertion.Assert(v)
			if err != nil {
				errs = append(errs, errors.WithQuery(err, queryutil.New().Index(i)))
			}
		}
		if len(errs) == 0 {
			return nil
		}
		if len(errs) == 1 {
			return errs[0]
		}
		return errors.Errors(errs...)
	})
}

// Or returns new assertion to ensure that value passes at least one of assertions.
// If the assertions are empty, it returns an error.
func Or(assertions ...Assertion) Assertion {
	return AssertionFunc(func(v interface{}) error {
		if len(assertions) == 0 {
			return errors.New("empty assertion list")
		}
		errs := []error{}
		for i, assertion := range assertions {
			assertion := assertion
			err := assertion.Assert(v)
			if err == nil {
				return nil
			}
			errs = append(errs, errors.WithQuery(err, queryutil.New().Index(i)))
		}
		if len(errs) == 1 {
			return errors.Wrap(errs[0], "all assertions failed")
		}
		return errors.Wrap(errors.Errors(errs...), "all assertions failed")
	})
}
