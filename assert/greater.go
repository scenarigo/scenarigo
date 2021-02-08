package assert

// Greater returns an assertion to ensure a value greater than the expected value.
func Greater(expected interface{}) Assertion {
	return AssertionFunc(func(actual interface{}) error {
		ok, err := compareNumber(actual, expected, compareGreater)
		if ok {
			return nil
		}
		return err
	})
}

// GreaterOrEqual returns an assertion to ensure a value equal or greater than the expected value.
func GreaterOrEqual(expected interface{}) Assertion {
	return AssertionFunc(func(actual interface{}) error {
		ok, err := compareNumber(actual, expected, compareGreaterOrEqual)
		if ok {
			return nil
		}
		return err
	})
}
