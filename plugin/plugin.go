package plugin

import (
	"fmt"
	"path/filepath"
	"plugin"
	"reflect"
	"strconv"
	"sync"
)

var (
	m         sync.Mutex
	cache     = map[string]Plugin{}
	newPlugin *openedPlugin
)

// Open opens a Go plugin.
// If a path has already been opened, then the existing *Plugin is returned.
// It is safe for concurrent use by multiple goroutines.
func Open(path string) (Plugin, error) {
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path: %w", err)
		}
		path = abs
	}
	filepath.Clean(path)
	m.Lock()
	defer m.Unlock()
	if p, ok := cache[path]; ok {
		return p, nil
	}
	newPlugin = &openedPlugin{} //nolint:exhaustruct
	defer func() { newPlugin = nil }()
	p, err := plugin.Open(path)
	if err != nil {
		return nil, err
	}
	newPlugin.Plugin = p
	cache[path] = newPlugin
	return newPlugin, nil
}

// Symbol is a pointer to a variable or function.
type Symbol = plugin.Symbol

// SetupFunc represents a setup function.
// If it returns non-nil teardown, the function will be called later.
type SetupFunc func(ctx *Context) (newCtx *Context, teardown func(*Context))

// Plugin represents a scenarigo plugin.
type Plugin interface {
	Lookup(name string) (Symbol, error)
	GetSetup() SetupFunc
	GetSetupEachScenario() SetupFunc
}

// RegisterSetup registers a function to setup for plugin.
// Plugins must call this function in their init function if it registers the setup process.
func RegisterSetup(setup SetupFunc) {
	if newPlugin == nil {
		panic("RegisterSetup must be called in init()")
	}
	newPlugin.m.Lock()
	defer newPlugin.m.Unlock()
	newPlugin.setups = append(newPlugin.setups, setup)
}

// RegisterSetupEachScenario registers a function to setup for plugin.
// Plugins must call this function in their init function if it registers the setup process.
// The registered function will be called before each scenario.
func RegisterSetupEachScenario(setup SetupFunc) {
	if newPlugin == nil {
		panic("RegisterSetupEachScenario must be called in init()")
	}
	newPlugin.m.Lock()
	defer newPlugin.m.Unlock()
	newPlugin.setupsEachScenario = append(newPlugin.setupsEachScenario, setup)
}

type openedPlugin struct {
	*plugin.Plugin
	m                  sync.Mutex
	setups             []SetupFunc
	setupsEachScenario []SetupFunc
}

// GetSetup implements Plugin interface.
func (p *openedPlugin) GetSetup() SetupFunc {
	p.m.Lock()
	defer p.m.Unlock()
	return p.getSetup(p.setups)
}

// GetSetupEachScenario implements Plugin interface.
func (p *openedPlugin) GetSetupEachScenario() SetupFunc {
	p.m.Lock()
	defer p.m.Unlock()
	return p.getSetup(p.setupsEachScenario)
}

func (p *openedPlugin) getSetup(setups []SetupFunc) SetupFunc {
	if len(setups) == 0 {
		return nil
	}
	if len(setups) == 1 {
		return setups[0]
	}
	return func(ctx *Context) (*Context, func(*Context)) {
		var teardowns []func(*Context)
		for i, setup := range setups {
			newCtx := ctx
			ctx.Run(strconv.Itoa(i+1), func(ctx *Context) {
				ctx, teardown := setup(ctx)
				if ctx != nil {
					newCtx = ctx
				}
				if teardown != nil {
					teardowns = append(teardowns, teardown)
				}
			})
			ctx = newCtx.WithReporter(ctx.Reporter())
		}
		if len(teardowns) == 0 {
			return ctx, nil
		}
		if len(teardowns) == 1 {
			return ctx, teardowns[0]
		}
		return ctx, func(ctx *Context) {
			for i, teardown := range teardowns {
				ctx.Run(strconv.Itoa(i+1), func(ctx *Context) {
					teardown(ctx)
				})
			}
		}
	}
}

// ExtractByKey implements query.KeyExtractor interface.
func (p *openedPlugin) ExtractByKey(key string) (any, bool) {
	sym, err := p.Lookup(key)
	if err != nil {
		return nil, false
	}
	// If sym is a pointer to a variable, return the actual variable for convenience.
	if v := reflect.ValueOf(sym); v.Kind() == reflect.Ptr {
		return v.Elem().Interface(), true
	}
	return sym, true
}
