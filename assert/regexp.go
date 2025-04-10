package assert

import (
	"errors"
	"fmt"
	"regexp"
)

// Regexp returns an assertion to ensure a value matches the regular expression pattern.
func Regexp(expr string) Assertion {
	pattern, err := regexp.Compile(expr)
	if err != nil {
		return AssertionFunc(func(v any) error {
			return err
		})
	}
	return AssertionFunc(func(v any) error {
		if s, ok := v.(string); ok {
			if pattern.MatchString(s) {
				return nil
			}
			return fmt.Errorf(`does not match the pattern "%s"`, expr)
		}

		s, err := convert(v, "")
		if err != nil {
			return errors.New("expect string")
		}

		if pattern.MatchString(s) {
			return nil
		}
		return fmt.Errorf(`does not match the pattern "%s"`, expr)
	})
}
