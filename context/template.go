package context

import "fmt"

// executeTemplateFunc is the function variable used to execute templates.
// It is registered by the template package via RegisterExecuteTemplateFunc
// to avoid a circular import from context → template.
var executeTemplateFunc func(c *Context, i any) (any, error)

// RegisterExecuteTemplateFunc registers the template execution function.
// The template package calls this in its init() to break the
// context → template circular import.
func RegisterExecuteTemplateFunc(f func(c *Context, i any) (any, error)) {
	executeTemplateFunc = f
}

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
	if executeTemplateFunc == nil {
		return nil, fmt.Errorf("template execution not registered; import the template package")
	}
	return executeTemplateFunc(c, i)
}
