// Package protocol provides defines APIs of protocol.
package protocol

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	query "github.com/zoncoen/query-go/v2"

	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/queryutil"
)

var (
	m        sync.Mutex
	registry = map[string]Protocol{}
)

// queryGoModulePrefix is the import-path prefix every major version of
// query-go shares. It is not a module path: v2's is the prefix plus /v2.
const queryGoModulePrefix = "github.com/zoncoen/query-go"

// looksLikeQueryOptions reports whether t has a method that was meant to be
// QueryOptions: no arguments and a single slice of a query-go Option. A method
// of that name with any other shape belongs to the protocol, not to this
// package, so it is left alone.
func looksLikeQueryOptions(t reflect.Type) bool {
	mt, ok := t.MethodByName("QueryOptions")
	if !ok {
		return false
	}
	ft := mt.Type
	// The receiver counts as the first input of a method obtained from a type.
	if ft.NumIn() != 1 || ft.NumOut() != 1 || ft.Out(0).Kind() != reflect.Slice {
		return false
	}
	elem := ft.Out(0).Elem()
	return isQueryGoOption(elem.Name(), elem.PkgPath())
}

// isQueryGoOption reports whether a named type is an Option that came from
// query-go, of any of its major versions. The type carries the path of the
// package that declares it, so an older query-go is recognised without
// depending on it - and depending on it would not help, since every version
// calls the type Option.
//
// Any package under query-go counts, not just its root: an Option from one of
// the extractor packages is not the Option the provider must return either, so
// a protocol returning those would have its options dropped just the same.
func isQueryGoOption(name, pkgPath string) bool {
	if name != "Option" {
		return false
	}
	return pkgPath == queryGoModulePrefix || strings.HasPrefix(pkgPath, queryGoModulePrefix+"/")
}

// Register registers the protocol to the registry.
//
// It panics when p has a QueryOptions method that looks like an attempt to
// implement QueryOptionsProvider but does not, which happens to a protocol
// still built against query-go v1.
func Register(p Protocol) {
	m.Lock()
	defer m.Unlock()
	pr, ok := p.(QueryOptionsProvider)
	if !ok {
		// A QueryOptions method with any other signature - typically the
		// []query-go/v1.Option one - compiles and registers, but its
		// options would be dropped without a trace and every assertion
		// against the protocol's responses would then extract the wrong
		// value. Refuse loudly, before the registry is touched.
		if looksLikeQueryOptions(reflect.TypeOf(p)) {
			panic(fmt.Sprintf("protocol %q: QueryOptions must return []query.Option of github.com/zoncoen/query-go/v2 to implement QueryOptionsProvider", p.Name()))
		}
	}
	registry[strings.ToLower(p.Name())] = p
	if ok {
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
