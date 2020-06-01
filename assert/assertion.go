// Package assert provides value assertions.
package assert

import (
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/zoncoen/query-go"
	"github.com/zoncoen/scenarigo/query/extractor"
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

func assertFunc(q *query.Query, f func(interface{}) error) Assertion {
	return AssertionFunc(func(v interface{}) error {
		got, err := q.Extract(v)
		if err != nil {
			return err
		}
		return f(got)
	})
}

// Build creates an assertion from Go value.
func Build(expect interface{}) Assertion {
	var assertions []Assertion
	if expect != nil {
		assertions = build(query.New(), expect)
	}
	return AssertionFunc(func(v interface{}) error {
		var assertErr error
		for _, assertion := range assertions {
			assertion := assertion
			if err := assertion.Assert(v); err != nil {
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
