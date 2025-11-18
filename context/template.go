package context

import (
	"fmt"

	"github.com/scenarigo/scenarigo/template"
)

// ExecuteTemplate executes template strings in context.
func ExecuteTemplate[T any](c *Context, v T) (T, error) {
	var t T
	vv, err := c.ExecuteTemplate(v)
	if err != nil {
		return t, err
	}
	t, ok := vv.(T)
	if !ok {
		return t, fmt.Errorf("expected %T but got %T", t, vv)
	}
	return t, nil
}

// ExecuteTemplate executes template strings in context.
func (c *Context) ExecuteTemplate(i any) (any, error) {
	ctx := template.WithExecutionContext(c.RequestContext(), c)
	return template.Execute(ctx, i, c)
}
