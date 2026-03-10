package context

import "context"

// newAssertionsFunc is the function variable used to create assertion helpers.
// It is registered by the assert package via RegisterNewAssertionsFunc
// to avoid a circular import from context → assert.
var newAssertionsFunc func(ctx context.Context) any

// RegisterNewAssertionsFunc registers the function that creates assertion
// helpers for template context. The assert package calls this in its init()
// to break the context → assert circular import.
func RegisterNewAssertionsFunc(f func(ctx context.Context) any) {
	newAssertionsFunc = f
}
