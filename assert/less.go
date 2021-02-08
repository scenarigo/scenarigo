package assert

// Less returns an assertion to ensure a value less than the expected value.
func Less(expected interface{}) Assertion {
	return AssertionFunc(func(actual interface{}) error {
		ok, err := compareNumber(actual, expected, compareLess)
		if ok {
			return nil
		}
		return err
	})
}

// LessOrEqual returns an assertion to ensure a value equal or less than the expected value.
func LessOrEqual(expected interface{}) Assertion {
	return AssertionFunc(func(actual interface{}) error {
		ok, err := compareNumber(actual, expected, compareLessOrEqual)
		if ok {
			return nil
		}
		return err
	})
}
