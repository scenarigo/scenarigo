// Package context provides the test context of scenarigo.
package context

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zoncoen/scenarigo/reporter"
)

type (
	keyPluginDir       struct{}
	keyPlugins         struct{}
	keyVars            struct{}
	keyRequest         struct{}
	keyResponse        struct{}
	keyResponseHeader  struct{}
	keyResponseTrailer struct{}
)

// Context represents a scenarigo context.
type Context struct {
	ctx      context.Context
	reqCtx   context.Context
	reporter reporter.Reporter
}

// New returns a new scenarigo context.
func New(r reporter.Reporter) *Context {
	return newContext(context.Background(), context.Background(), r)
}

// FromT creates a new context from t.
func FromT(t *testing.T) *Context {
	return newContext(context.Background(), context.Background(), reporter.FromT(t))
}

// nolint:golint
func newContext(ctx context.Context, reqCtx context.Context, r reporter.Reporter) *Context {
	return &Context{
		ctx:      ctx,
		reqCtx:   reqCtx,
		reporter: r,
	}
}

// WithRequestContext returns the context.Context for request.
func (c *Context) WithRequestContext(reqCtx context.Context) *Context {
	return newContext(
		c.ctx,
		reqCtx,
		c.reporter,
	)
}

// RequestContext returns the context.Context for request.
func (c *Context) RequestContext() context.Context {
	return c.reqCtx
}

// WithReporter returns a copy of c with new test reporter.
func (c *Context) WithReporter(r reporter.Reporter) *Context {
	return newContext(c.ctx, c.reqCtx, r)
}

// Reporter returns the reporter of context.
func (c *Context) Reporter() reporter.Reporter {
	return c.reporter
}

// WithPluginDir returns a copy of c with plugin root directory.
func (c *Context) WithPluginDir(path string) *Context {
	abs, err := filepath.Abs(path)
	if err != nil {
		c.Reporter().Fatalf("failed to get absolute path: %s", err)
	}
	return newContext(
		context.WithValue(c.ctx, keyPluginDir{}, abs),
		c.reqCtx,
		c.reporter,
	)
}

// PluginDir returns the plugins root directory.
func (c *Context) PluginDir() string {
	path, ok := c.ctx.Value(keyPluginDir{}).(string)
	if ok {
		return path
	}
	return ""
}

// WithPlugins returns a copy of c with ps.
func (c *Context) WithPlugins(ps map[string]interface{}) *Context {
	if ps == nil {
		return c
	}
	plugins, _ := c.ctx.Value(keyPlugins{}).(Plugins)
	plugins = plugins.Append(ps)
	return newContext(
		context.WithValue(c.ctx, keyPlugins{}, plugins),
		c.reqCtx,
		c.reporter,
	)
}

// Plugins returns the plugins.
func (c *Context) Plugins() Plugins {
	ps, ok := c.ctx.Value(keyPlugins{}).(Plugins)
	if ok {
		return ps
	}
	return nil
}

// WithVars returns a copy of c with v.
func (c *Context) WithVars(v interface{}) *Context {
	if v == nil {
		return c
	}
	vars, _ := c.ctx.Value(keyVars{}).(Vars)
	vars = vars.Append(v)
	return newContext(
		context.WithValue(c.ctx, keyVars{}, vars),
		c.reqCtx,
		c.reporter,
	)
}

// Vars returns the context variables.
func (c *Context) Vars() Vars {
	vs, ok := c.ctx.Value(keyVars{}).(Vars)
	if ok {
		return vs
	}
	return nil
}

// WithRequest returns a copy of c with request.
func (c *Context) WithRequest(req interface{}) *Context {
	if req == nil {
		return c
	}
	return newContext(
		context.WithValue(c.ctx, keyRequest{}, req),
		c.reqCtx,
		c.reporter,
	)
}

// Request returns the request.
func (c *Context) Request() interface{} {
	return c.ctx.Value(keyRequest{})
}

// WithResponse returns a copy of c with response.
// If you want to pass response with http protocol,
// trailer argument should pass nil because it currently supported gRPC only.
// ( more detail: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Trailer )
func (c *Context) WithResponse(resp interface{}, header interface{}, trailer interface{}) *Context {
	if resp == nil {
		return c
	}
	ctx := context.WithValue(c.ctx, keyResponse{}, resp)
	if header != nil {
		ctx = context.WithValue(ctx, keyResponseHeader{}, header)
	}
	if trailer != nil {
		ctx = context.WithValue(ctx, keyResponseTrailer{}, trailer)
	}
	return newContext(ctx, c.reqCtx, c.reporter)
}

// Response returns the response.
func (c *Context) Response() interface{} {
	return c.ctx.Value(keyResponse{})
}

// ResponseHeader returns header of response.
func (c *Context) ResponseHeader() interface{} {
	return c.ctx.Value(keyResponseHeader{})
}

// ResponseTrailer returns trailer of response.
func (c *Context) ResponseTrailer() interface{} {
	return c.ctx.Value(keyResponseTrailer{})
}

// Run runs f as a subtest of c called name.
func (c *Context) Run(name string, f func(*Context)) bool {
	return c.Reporter().Run(name, func(r reporter.Reporter) { f(c.WithReporter(r)) })
}
