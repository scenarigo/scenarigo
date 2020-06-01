package context

import (
	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"

	"github.com/zoncoen/scenarigo/assert"
)

var assertions = map[string]interface{}{
	"notZero":     assert.NotZero,
	"contains":    leftArrowFunc(assert.Contains),
	"notContains": leftArrowFunc(assert.NotContains),
}

type leftArrowFunc func(assertion assert.Assertion) func(*query.Query) assert.Assertion

func (f leftArrowFunc) Exec(arg interface{}) (interface{}, error) {
	assertion, ok := arg.(assert.Assertion)
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
	return assert.Build(i), nil
}
