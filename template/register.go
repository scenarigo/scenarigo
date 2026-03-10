package template

import sccontext "github.com/scenarigo/scenarigo/context"

func init() {
	sccontext.RegisterExecuteTemplateFunc(func(c *sccontext.Context, i any) (any, error) {
		ctx := WithExecutionContext(c.RequestContext(), c)
		return Execute(ctx, i, c)
	})
}
