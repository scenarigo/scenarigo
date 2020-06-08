package context

const (
	nameContext  = "ctx"
	namePlugins  = "plugins"
	nameVars     = "vars"
	nameRequest  = "request"
	nameResponse = "response"
	nameEnv      = "env"
	nameAssert   = "assert"
)

var assertions map[string]interface{}

func RegisterAssertions(asserts map[string]interface{}) {
	assertions = asserts
}

// ExtractByKey implements query.KeyExtractor interface.
func (c *Context) ExtractByKey(key string) (interface{}, bool) {
	switch key {
	case nameContext:
		return c, true
	case namePlugins:
		v := c.Plugins()
		if v != nil {
			return v, true
		}
	case nameVars:
		v := c.Vars()
		if v != nil {
			return v, true
		}
	case nameRequest:
		v := c.Request()
		if v != nil {
			return v, true
		}
	case nameResponse:
		v := c.Response()
		if v != nil {
			return v, true
		}
	case nameEnv:
		return env, true
	case nameAssert:
		return assertions, true
	}
	return nil, false
}
