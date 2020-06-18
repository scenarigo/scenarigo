package context

import (
	"github.com/zoncoen/scenarigo/template"
)

// ExecuteTemplate executes template strings in context.
// nolint:stylecheck
func (ctx *Context) ExecuteTemplate(i interface{}) (interface{}, error) {
	if ctx.DryRun() {
		return i, nil
	}
	return template.Execute(i, ctx)
}
