// Package protocol provides defines APIs of protocol.
package protocol

import (
	"strings"
	"sync"

	"github.com/zoncoen/query-go"

	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/queryutil"
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
	if pr, ok := p.(QueryOptionsProvider); ok {
		queryutil.AppendOptions(pr.QueryOptions()...)
	}
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
	UnmarshalOption([]byte) error
	UnmarshalRequest([]byte) (Invoker, error)
	UnmarshalExpect([]byte) (AssertionBuilder, error)
}

// Invoker is the interface that sends the request and returns response sent from the server.
type Invoker interface {
	Invoke(*context.Context) (*context.Context, any, error)
}

// AssertionBuilder builds the assertion for the result of Invoke.
type AssertionBuilder interface {
	Build(*context.Context) (assert.Assertion, error)
}

// QueryOptionsProvider is the interface that provides custom querying options.
type QueryOptionsProvider interface {
	QueryOptions() []query.Option
}
