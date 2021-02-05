package assert

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"

	"github.com/zoncoen/query-go"
)

var typeString = reflect.TypeOf("")

// Regexp returns an assertion to ensure a value matches the regular expression pattern.
func Regexp(expr string) func(*query.Query) Assertion {
	pattern, err := regexp.Compile(expr)
	if err != nil {
		return func(q *query.Query) Assertion {
			return assertFunc(q, func(v interface{}) error {
				return err
			})
		}
	}
	return func(q *query.Query) Assertion {
		return assertFunc(q, func(v interface{}) error {
			if s, ok := v.(string); ok {
				if pattern.MatchString(s) {
					return nil
				}
				return fmt.Errorf(`does not match the pattern "%s"`, expr)
			}

			converted, err := convertComparableValue(v, typeString)
			if err == nil {
				if s, ok := converted.(string); ok {
					if pattern.MatchString(s) {
						return nil
					}
					return fmt.Errorf(`does not match the pattern "%s"`, expr)
				}
			}

			return errors.New("expect string")
		})
	}
}
