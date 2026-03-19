package assert

// Greater returns an assertion to ensure a value greater than the expected value.
func Greater(expected any) Assertion {
	return AssertionFunc(func(actual any) error {
		return compareNumber(actual, expected, compareGreater)
	})
}

// GreaterOrEqual returns an assertion to ensure a value equal or greater than the expected value.
func GreaterOrEqual(expected any) Assertion {
	return AssertionFunc(func(actual any) error {
		return compareNumber(actual, expected, compareGreaterOrEqual)
	})
}
