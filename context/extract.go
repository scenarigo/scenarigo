package context

import (
	"context"

	query "github.com/zoncoen/query-go/v2"
)

const (
	nameContext  = "ctx"
	namePlugins  = "plugins"
	nameVars     = "vars"
	nameSecrets  = "secrets"
	nameSteps    = "steps"
	nameRequest  = "request"
	nameResponse = "response"
	nameEnv      = "env"
	nameAssert   = "assert"
)

var _ query.KeyExtractor = (*Context)(nil)

// ExtractByKey implements query.KeyExtractor interface.
func (c *Context) ExtractByKey(_ context.Context, key string) (any, error) {
	switch key {
	case nameContext:
		return c, nil
	case namePlugins:
		v := c.Plugins()
		if v != nil {
			return v, nil
		}
	case nameVars:
		v := c.Vars()
		if v != nil {
			return v, nil
		}
	case nameSecrets:
		v := c.Secrets()
		if v != nil {
			return v, nil
		}
	case nameSteps:
		v := c.Steps()
		if v != nil {
			return v, nil
		}
	case nameRequest:
		v := c.Request()
		if v != nil {
			return v, nil
		}
	case nameResponse:
		v := c.Response()
		if v != nil {
			return v, nil
		}
	case nameEnv:
		return env, nil
	case nameAssert:
		if newAssertionsFunc != nil {
			return newAssertionsFunc(c.RequestContext()), nil
		}
	}
	return nil, query.ErrNotFound
}
