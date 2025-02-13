package protocol

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/scenarigo/scenarigo/logger"
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

// All returns all registered protocol.
func All() map[string]Protocol {
	m.Lock()
	defer m.Unlock()
	protocols := map[string]Protocol{}
	for name, p := range registry {
		protocols[name] = p
	}
	return protocols
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

// Protocol is the interface that creates mock server.
type Protocol interface {
	Name() string
	UnmarshalConfig([]byte) (interface{}, error)
	NewServer(iter *MockIterator, l logger.Logger, config interface{}) (Server, error)
}

// Server represents a mock server.
type Server interface {
	Start(context.Context) error
	Wait(context.Context) error
	Stop(context.Context) error
	Addr() (string, error)
}

// ErrServerClosed is the error that the server is already closed.
var ErrServerClosed = errors.New("server closed")
