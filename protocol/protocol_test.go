package protocol

import (
	"errors"
	"strings"
	"testing"

	query "github.com/zoncoen/query-go/v2"
)

type stubProtocol struct{ name string }

func (p *stubProtocol) Name() string                             { return p.name }
func (p *stubProtocol) UnmarshalOption([]byte) error             { return nil }
func (p *stubProtocol) UnmarshalRequest([]byte) (Invoker, error) { return nil, errors.ErrUnsupported }
func (p *stubProtocol) UnmarshalExpect([]byte) (AssertionBuilder, error) {
	return nil, errors.ErrUnsupported
}

// optionsProtocol implements QueryOptionsProvider.
type optionsProtocol struct{ stubProtocol }

func (p *optionsProtocol) QueryOptions() []query.Option { return nil }

// staleOptionsProtocol has a QueryOptions method that returns query-go options
// but does not match QueryOptionsProvider, so its options would be dropped. A
// protocol built against query-go v1 is in this position; a named result type
// reproduces it without depending on v1, which every version of query-go calls
// Option anyway.
type queryOptions []query.Option

type staleOptionsProtocol struct{ stubProtocol }

func (p *staleOptionsProtocol) QueryOptions() queryOptions { return nil }

// unrelatedOptionsProtocol has a method that happens to be called QueryOptions
// but was never meant to implement QueryOptionsProvider.
type unrelatedOptionsProtocol struct{ stubProtocol }

func (p *unrelatedOptionsProtocol) QueryOptions() []string { return nil }

func TestRegister(t *testing.T) {
	t.Run("plain protocol", func(t *testing.T) {
		p := &stubProtocol{name: "plain"}
		Register(p)
		defer Unregister(p.Name())
		if got := Get("PLAIN"); got != Protocol(p) {
			t.Fatalf("expected the registered protocol but got %v", got)
		}
	})
	t.Run("QueryOptionsProvider", func(t *testing.T) {
		p := &optionsProtocol{stubProtocol{name: "options"}}
		Register(p)
		defer Unregister(p.Name())
		if got := Get(p.Name()); got != Protocol(p) {
			t.Fatalf("expected the registered protocol but got %v", got)
		}
	})
	t.Run("stale QueryOptions signature", func(t *testing.T) {
		p := &staleOptionsProtocol{stubProtocol{name: "stale"}}
		defer Unregister(p.Name())
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected Register to panic")
			}
			msg, ok := r.(string)
			if !ok || !strings.Contains(msg, `protocol "stale": QueryOptions must return []query.Option`) {
				t.Fatalf("unexpected panic: %v", r)
			}
			if got := Get(p.Name()); got != nil {
				t.Fatalf("the refused protocol was registered: %v", got)
			}
		}()
		Register(p)
	})
	t.Run("unrelated QueryOptions method", func(t *testing.T) {
		// Refusing a protocol only because a method shares the name would take
		// down the process at init time for something that has nothing to do
		// with query-go.
		p := &unrelatedOptionsProtocol{stubProtocol{name: "unrelated"}}
		defer Unregister(p.Name())
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("unexpected panic: %v", r)
			}
		}()
		Register(p)
		if got := Get(p.Name()); got != Protocol(p) {
			t.Fatalf("expected the registered protocol but got %v", got)
		}
	})
	t.Run("isQueryGoOption", func(t *testing.T) {
		for _, test := range []struct {
			name, pkgPath string
			expect        bool
		}{
			{"Option", "github.com/zoncoen/query-go", true},
			{"Option", "github.com/zoncoen/query-go/v2", true},
			{"Option", "github.com/zoncoen/query-golang", false},
			{"Option", "example.com/other", false},
			{"Options", "github.com/zoncoen/query-go", false},
			{"string", "", false},
		} {
			if got := isQueryGoOption(test.name, test.pkgPath); got != test.expect {
				t.Errorf("isQueryGoOption(%q, %q) = %v", test.name, test.pkgPath, got)
			}
		}
	})
}
