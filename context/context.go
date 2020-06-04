// Package context provides the test context of scenarigo.
package context

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/zoncoen/scenarigo/reporter"
)

type (
	keyPluginDir struct{}
	keyPlugins   struct{}
	keyVars      struct{}
	keyRequest   struct{}
	keyResponse  struct{}
	keyYAML      struct{}
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
func (c *Context) WithResponse(resp interface{}) *Context {
	if resp == nil {
		return c
	}
	return newContext(
		context.WithValue(c.ctx, keyResponse{}, resp),
		c.reqCtx,
		c.reporter,
	)
}

// Response returns the response.
func (c *Context) Response() interface{} {
	return c.ctx.Value(keyResponse{})
}

type YAML struct {
	ScenarioPath string
	File         *ast.File
	Builder      *yaml.PathBuilder
}

func NewYAML(path string) (*YAML, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file, err := parser.ParseBytes(bytes, 0)
	if err != nil {
		return nil, err
	}
	return &YAML{
		ScenarioPath: path,
		File:         file,
		Builder:      &yaml.PathBuilder{},
	}, nil
}

func (c *Context) AddChildPath(selector string) *Context {
	yml, ok := c.ctx.Value(keyYAML{}).(*YAML)
	if !ok {
		return c
	}
	b := yml.Builder.Child(selector)
	yml.Builder = &(*b) // copy PathBuilder
	return c
}

func (c *Context) WithYAML(yml *YAML) *Context {
	if yml == nil {
		return c
	}
	return newContext(
		context.WithValue(c.ctx, keyYAML{}, yml),
		c.reqCtx,
		c.reporter,
	)
}

// Run runs f as a subtest of c called name.
func (c *Context) Run(name string, f func(*Context)) bool {
	return c.Reporter().Run(name, func(r reporter.Reporter) { f(c.WithReporter(r)) })
}
