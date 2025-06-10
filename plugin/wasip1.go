//go:build wasip1

package plugin

import (
	"encoding/json"
	"fmt"
	"plugin"
	"strconv"

	"github.com/scenarigo/scenarigo/context"
)

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

var (
	setups             []SetupFunc
	setupsEachScenario []SetupFunc
	teardowns          []func(*Context)
)

// RegisterSetup registers a function to setup for plugin.
// Plugins must call this function in their init function if it registers the setup process.
func RegisterSetup(setup SetupFunc) {
	setups = append(setups, setup)
}

// RegisterSetupEachScenario registers a function to setup for plugin.
// Plugins must call this function in their init function if it registers the setup process.
// The registered function will be called before each scenario.
func RegisterSetupEachScenario(setup SetupFunc) {
	setupsEachScenario = append(setupsEachScenario, setup)
}

func RunSetup(arg string) {
	var sc context.SerializableContext
	if err := json.Unmarshal([]byte(arg), &sc); err != nil {
		fmt.Println("failed to decode", err)
		return
	}
	ctx := context.FromSerializable(&sc)
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
}

func RunTeardown(arg string) {
	var sc context.SerializableContext
	if err := json.Unmarshal([]byte(arg), &sc); err != nil {
		fmt.Println("failed to decode", err)
		return
	}
	ctx := context.FromSerializable(&sc)
	for i, teardown := range teardowns {
		ctx.Run(strconv.Itoa(i+1), func(ctx *Context) {
			teardown(ctx)
		})
	}
}
