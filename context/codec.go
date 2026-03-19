package context

import (
	"path/filepath"

	"github.com/scenarigo/scenarigo/color"
	"github.com/scenarigo/scenarigo/reporter"
)

// QueryValue represents a query value with its string representation.
// It is used for serializing query-value pairs when converting context data.
type QueryValue struct {
	Query string `json:"query"`
	Value string `json:"value"`
}

// SerializableSecrets represents a serializable version of Secrets.
// It contains both raw secrets data and their processed query values.
type SerializableSecrets struct {
	Secrets []any        `json:"secrets"`
	Values  []QueryValue `json:"values"`
}

// SerializableContext represents a serializable version of Context.
// It contains all the necessary context data that can be marshaled to JSON
// for communication between host and WASM plugins.
type SerializableContext struct {
	ScenarioFilepath string                         `json:"scenarioFilepath,omitempty"`
	PluginDir        string                         `json:"pluginDir,omitempty"`
	Plugins          []map[string]any               `json:"plugins,omitempty"`
	Vars             []any                          `json:"vars,omitempty"`
	Secrets          *SerializableSecrets           `json:"secrets,omitempty"`
	Steps            *Steps                         `json:"steps,omitempty"`
	Request          any                            `json:"request,omitempty"`
	Response         any                            `json:"response,omitempty"`
	ColorEnabled     bool                           `json:"colorEnabled"`
	Reporter         *reporter.SerializableReporter `json:"reporter,omitempty"`
	// ReporterID is used to look up the reference to the reporter from the ReporterMap.
	// The value of ReporterID is determined in the reporter's ToSerializable method.
	ReporterID string `json:"reporterId"`
	// The reporter has parent and children fields, which are processed recursively.
	// Since references can be circular, a ReporterMap is used to manage the references.
	ReporterMap map[string]*reporter.SerializableReporter `json:"reporterMap"`
}

// ToSerializable converts Context to SerializableContext.
// This method serializes the context data for transmission to WASM plugins.
func (c *Context) ToSerializable() *SerializableContext {
	sc := &SerializableContext{
		ScenarioFilepath: c.ScenarioFilepath(),
		PluginDir:        c.PluginDir(),
	}

	// Store ColorConfig enabled state for WASM plugins
	if colorConfig := c.ColorConfig(); colorConfig != nil {
		sc.ColorEnabled = colorConfig.IsEnabled()
	}

	// Convert plugins
	if plugins := c.Plugins(); plugins != nil {
		sc.Plugins = plugins
	}

	// Convert vars
	if vars := c.Vars(); vars != nil {
		sc.Vars = vars
	}

	// Convert secrets
	if secrets := c.Secrets(); secrets != nil {
		queryValues := make([]QueryValue, len(secrets.values))
		for i, v := range secrets.values {
			queryValues[i] = QueryValue{
				Query: v.query,
				Value: v.v,
			}
		}
		sc.Secrets = &SerializableSecrets{
			Secrets: secrets.secrets,
			Values:  queryValues,
		}
	}

	// Convert steps
	if steps := c.Steps(); steps != nil {
		sc.Steps = steps
	}

	// Convert request and response
	if req := c.Request(); req != nil {
		sc.Request = req
	}
	if resp := c.Response(); resp != nil {
		sc.Response = resp
	}

	// Convert reporter if it supports serialization
	if r, ok := c.Reporter().(interface {
		ToSerializable(map[string]*reporter.SerializableReporter) string
	}); ok {
		reporterMap := make(map[string]*reporter.SerializableReporter)
		sc.ReporterID = r.ToSerializable(reporterMap)
		sc.ReporterMap = reporterMap
	}

	return sc
}

// FromSerializable creates a new Context from SerializableContext.
// This function reconstructs a context from serialized data received from WASM plugins.
func FromSerializable(sc *SerializableContext) (*Context, error) {
	return FromSerializableWithContext(New(nil), sc)
}

// FromSerializableWithContext applies the result of SerializableContext to the given Context.
func FromSerializableWithContext(ctx *Context, sc *SerializableContext) (*Context, error) {
	// Set reporter if it was serialized.
	if sc.ReporterID != "" {
		r, err := reporter.FromSerializableWithReporter(ctx.reporter, sc.ReporterID, sc.ReporterMap)
		if err != nil {
			return nil, err
		}
		ctx.reporter = r
	}

	// Set scenario filepath.
	if sc.ScenarioFilepath != "" {
		ctx = ctx.WithScenarioFilepath(sc.ScenarioFilepath)
	}

	// Set plugin directory.
	if sc.PluginDir != "" {
		abs, err := filepath.Abs(sc.PluginDir)
		if err != nil {
			// Since we can't return error, we'll use the original path.
			ctx = ctx.WithPluginDir(sc.PluginDir)
		} else {
			ctx = ctx.WithPluginDir(abs)
		}
	}

	// Set plugins
	if sc.Plugins != nil {
		for _, plg := range sc.Plugins {
			ctx = ctx.WithPlugins(plg)
		}
	}

	// Set vars
	if sc.Vars != nil {
		for _, v := range sc.Vars {
			ctx = ctx.WithVars(v)
		}
	}

	// Set secrets
	if sc.Secrets != nil {
		queryValues := make([]queryValue, len(sc.Secrets.Values))
		for i, v := range sc.Secrets.Values {
			queryValues[i] = queryValue{
				query: v.Query,
				v:     v.Value,
			}
		}
		secrets := &Secrets{
			secrets: sc.Secrets.Secrets,
			values:  queryValues,
		}
		ctx = ctx.WithSecrets(secrets)
	}

	// Set steps
	if sc.Steps != nil {
		ctx = ctx.WithSteps(sc.Steps)
	}

	// Set request and response
	if sc.Request != nil {
		ctx = ctx.WithRequest(sc.Request)
	}
	if sc.Response != nil {
		ctx = ctx.WithResponse(sc.Response)
	}

	// Restore ColorConfig with preserved enabled state
	colorConfig := color.New()
	colorConfig.SetEnabled(sc.ColorEnabled)
	ctx = ctx.WithColorConfig(colorConfig)

	return ctx, nil
}
