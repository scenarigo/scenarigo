package protocol

import (
	"context"
	"errors"
	"testing"

	"github.com/scenarigo/scenarigo/logger"
)

func TestRegistry(t *testing.T) {
	p := &testProtocol{}
	if l := len(All()); l != 0 {
		t.Fatal("failed to register")
	}
	if got := Get(p.Name()); got != nil {
		t.Fatal("already registered")
	}
	Register(p)
	if l := len(All()); l != 1 {
		t.Fatal("failed to register")
	}
	if got := Get(p.Name()); got == nil {
		t.Fatal("failed to register")
	}
}

type testProtocol struct{}

func (_ *testProtocol) Name() string { return "test" } //nolint:revive

func (_ *testProtocol) UnmarshalConfig(b []byte) (interface{}, error) { //nolint:revive
	return nil, nil //nolint:nilnil
}

func (_ *testProtocol) NewServer(iter *MockIterator, l logger.Logger, cfg interface{}) (Server, error) { //nolint:revive
	if iter == nil {
		return nil, errors.New("mock iterator is nil")
	}
	return &testServer{}, nil
}

type testServer struct{}

func (_ testServer) Start(ctx context.Context) error { return nil }     //nolint:revive
func (_ testServer) Wait(ctx context.Context) error  { return nil }     //nolint:revive
func (_ testServer) Stop(ctx context.Context) error  { return nil }     //nolint:revive
func (_ testServer) Addr() (string, error)           { return "", nil } //nolint:revive
