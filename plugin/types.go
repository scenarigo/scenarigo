package plugin

import "plugin"

// Symbol is a pointer to a variable or function.
type Symbol = plugin.Symbol

// Plugin represents a scenarigo plugin.
// It provides an interface for both native Go plugins and WASM plugins.
type Plugin interface {
	Lookup(name string) (Symbol, error)
	GetSetup() SetupFunc
	GetSetupEachScenario() SetupFunc
	Close() error
}
