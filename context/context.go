// Package context provides the test context of scenarigo.
package context

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml/ast"
	"github.com/scenarigo/scenarigo/color"
	"github.com/scenarigo/scenarigo/reporter"
)

type (
	keyScenarioName     struct{}
	keyScenarioFilepath struct{}
	keyPluginDir        struct{}
	keyPlugins          struct{}
	keyVars             struct{}
	keySecrets          struct{}
	keySteps            struct{}
	keyRequest          struct{}
	keyResponse         struct{}
	keyYAMLNode         struct{}
	keyColorConfig      struct{}
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
	t.Helper()
	return newContext(context.Background(), context.Background(), reporter.FromT(t))
}

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
	if s := c.Secrets(); s != nil {
		reporter.SetLogReplacer(c.reporter, s)
	}
	return newContext(c.ctx, c.reqCtx, r)
}

// Reporter returns the reporter of context.
func (c *Context) Reporter() reporter.Reporter {
	return c.reporter
}

// WithScenarioTitle returns a copy of c with scenario title.
func (c *Context) WithScenarioTitle(title string) *Context {
	return newContext(
		context.WithValue(c.ctx, keyScenarioName{}, title),
		c.reqCtx,
		c.reporter,
	)
}

// ScenarioTitle returns the title of the scenario executing in this context.
func (c *Context) ScenarioTitle() string {
	title, ok := c.ctx.Value(keyScenarioName{}).(string)
	if ok {
		return title
	}
	return ""
}

// WithScenarioFilepath returns a copy of c with the scenario filepath.
func (c *Context) WithScenarioFilepath(path string) *Context {
	return newContext(
		context.WithValue(c.ctx, keyScenarioFilepath{}, path),
		c.reqCtx,
		c.reporter,
	)
}

// ScenarioFilepath returns the filepath of the scenario executing in this context.
func (c *Context) ScenarioFilepath() string {
	path, ok := c.ctx.Value(keyScenarioFilepath{}).(string)
	if ok {
		return path
	}
	return ""
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
func (c *Context) WithPlugins(ps map[string]any) *Context {
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
func (c *Context) WithVars(v any) *Context {
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

// WithSecrets returns a copy of c with v.
func (c *Context) WithSecrets(s any) *Context {
	if s == nil {
		return c
	}
	secrets, _ := c.ctx.Value(keySecrets{}).(*Secrets)
	secrets = secrets.Append(s)
	reporter.SetLogReplacer(c.reporter, secrets)
	return newContext(
		context.WithValue(c.ctx, keySecrets{}, secrets),
		c.reqCtx,
		c.reporter,
	)
}

// Secrets returns the context secrets.
func (c *Context) Secrets() *Secrets {
	secrets, ok := c.ctx.Value(keySecrets{}).(*Secrets)
	if ok {
		return secrets
	}
	return nil
}

// WithSteps returns a copy of c with steps.
func (c *Context) WithSteps(steps *Steps) *Context {
	if steps == nil {
		return c
	}
	return newContext(
		context.WithValue(c.ctx, keySteps{}, steps),
		c.reqCtx,
		c.reporter,
	)
}

// Steps returns the steps.
func (c *Context) Steps() *Steps {
	v, ok := c.ctx.Value(keySteps{}).(*Steps)
	if ok {
		return v
	}
	return nil
}

// WithRequest returns a copy of c with request.
func (c *Context) WithRequest(req any) *Context {
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
func (c *Context) Request() any {
	return c.ctx.Value(keyRequest{})
}

// WithResponse returns a copy of c with response.
func (c *Context) WithResponse(resp any) *Context {
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
func (c *Context) Response() any {
	return c.ctx.Value(keyResponse{})
}

// WithNode returns a copy of c with ast.Node.
func (c *Context) WithNode(node ast.Node) *Context {
	if node == nil {
		return c
	}
	return newContext(
		context.WithValue(c.ctx, keyYAMLNode{}, node),
		c.reqCtx,
		c.reporter,
	)
}

// Node returns the ast.Node.
func (c *Context) Node() ast.Node {
	node, ok := c.ctx.Value(keyYAMLNode{}).(ast.Node)
	if !ok {
		return nil
	}
	return node
}

// WithColorConfig returns a copy of c with color configuration.
func (c *Context) WithColorConfig(colorConfig *color.Config) *Context {
	return newContext(
		context.WithValue(c.ctx, keyColorConfig{}, colorConfig),
		c.reqCtx,
		c.reporter,
	)
}

// ColorConfig returns the color configuration if available.
func (c *Context) ColorConfig() *color.Config {
	colorConfig, ok := c.ctx.Value(keyColorConfig{}).(*color.Config)
	if ok {
		return colorConfig
	}
	return nil
}

// Run runs f as a subtest of c called name.
func (c *Context) Run(name string, f func(*Context)) bool {
	return c.Reporter().Run(name, func(r reporter.Reporter) { f(c.WithReporter(r)) })
}

// RunWithRetry runs f as a subtest of c called name with retry.
func RunWithRetry(c *Context, name string, f func(*Context), policy reporter.RetryPolicy) bool {
	reqCtx := c.RequestContext()
	return reporter.RunWithRetry(reqCtx, c.Reporter(), name, func(r reporter.Reporter) {
		reqCtx, cancel := context.WithCancel(reqCtx)
		defer cancel()
		f(c.WithRequestContext(reqCtx).WithReporter(r))
	}, policy)
}

// Teardown registers a named function to be called when all parallel subtests complete.
func (c *Context) Teardown(name string, f func(*Context)) {
	c.Reporter().Teardown(name, func(r reporter.Reporter) {
		f(c.WithRequestContext(context.WithoutCancel(c.reqCtx)).WithReporter(r))
	})
}
