package assert

// Less returns an assertion to ensure a value less than the expected value.
func Less(expected interface{}) Assertion {
	return AssertionFunc(func(actual interface{}) error {
		return compareNumber(actual, expected, compareLess)
	})
}

// LessOrEqual returns an assertion to ensure a value equal or less than the expected value.
func LessOrEqual(expected interface{}) Assertion {
	return AssertionFunc(func(actual interface{}) error {
		return compareNumber(actual, expected, compareLessOrEqual)
	})
}
