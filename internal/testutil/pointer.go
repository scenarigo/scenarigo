package testutil

// ToPtr returns the pointer to t.
func ToPtr[T any](t T) *T {
	return &t
}
