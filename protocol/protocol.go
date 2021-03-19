// Package protocol provides defines APIs of protocol.
package protocol

import (
	"strings"
	"sync"

	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
)

var (
	m        sync.Mutex
	registry = map[string]Protocol{}
)

// Register registers the protocol to the registry.
func Register(p Protocol) {
	m.Lock()
	defer m.Unlock()
	registry[strings.ToLower(p.Name())] = p
}

// Unregister unregisters the protocol from the registry.
func Unregister(name string) {
	m.Lock()
	defer m.Unlock()
	delete(registry, strings.ToLower(name))
}

// Get returns the protocol registered with the given name.
func Get(name string) Protocol {
	m.Lock()
	defer m.Unlock()
	p, ok := registry[strings.ToLower(name)]
	if !ok {
		return nil
	}
	return p
}

// Protocol is the interface that creates Invoker and AssertionBuilder from YAML.
type Protocol interface {
	Name() string
	UnmarshalRequest([]byte) (Invoker, error)
	UnmarshalExpect([]byte) (AssertionBuilder, error)
}

// Invoker is the interface that sends the request and returns response sent from the server.
type Invoker interface {
	Invoke(*context.Context) (*context.Context, interface{}, error)
}

// InvokerFunc is an adaptor to allow the use of ordinary functions as Invoker.
func InvokerFunc(f func(*context.Context) (*context.Context, interface{}, error)) Invoker {
	return invoker(f)
}

type invoker func(*context.Context) (*context.Context, interface{}, error)

// Invoke implements Invoker interface.
func (f invoker) Invoke(ctx *context.Context) (*context.Context, interface{}, error) {
	return f(ctx)
}

// AssertionBuilder builds the assertion for the result of Invoke.
type AssertionBuilder interface {
	Build(*context.Context) (assert.Assertion, error)
}

// AssertionBuilderFunc is an adaptor to allow the use of ordinary functions as AssertionBuilder.
func AssertionBuilderFunc(f func(*context.Context) (assert.Assertion, error)) AssertionBuilder {
	return assertionBuilder(f)
}

type assertionBuilder func(*context.Context) (assert.Assertion, error)

// Build implements AssertionBuilder interface.
func (f assertionBuilder) Build(ctx *context.Context) (assert.Assertion, error) {
	return f(ctx)
}
