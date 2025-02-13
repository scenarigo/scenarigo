package mock

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/scenarigo/scenarigo/internal/yamlutil"
	"github.com/scenarigo/scenarigo/logger"
	"github.com/scenarigo/scenarigo/mock/protocol"
)

func TestNewServer(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Run("w/o http config", func(t *testing.T) {
			f, err := os.Open("protocol/http/testdata/http.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			var mocks []protocol.Mock
			if err := yaml.NewDecoder(f).Decode(&mocks); err != nil {
				t.Fatal(err)
			}
			srv, err := NewServer(
				&ServerConfig{
					Mocks: mocks,
				},
				logger.NewNopLogger(),
			)
			if err != nil {
				t.Fatalf("failed to create server: %s", err)
			}
			ch := make(chan error)
			go func() {
				ch <- srv.Start(context.Background())
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := srv.Wait(ctx); err != nil {
				t.Errorf("failed to wait: %s", err)
			}
			addrs, err := srv.Addrs()
			if err != nil {
				t.Errorf("failed to get addresses: %s", err)
			}
			resp, err := http.Get(fmt.Sprintf("http://%s", addrs["http"]))
			if err != nil {
				t.Errorf("failed to request: %s", err)
			} else {
				if got, expect := resp.StatusCode, 200; got != expect {
					t.Errorf("expect %d but got %d", expect, got)
				}
			}
			defer resp.Body.Close()
			if err := srv.Stop(ctx); err != nil {
				t.Errorf("failed to stop: %s", err)
			}
			if err := <-ch; err != nil {
				t.Errorf("failed to start: %s", err)
			}
		})
		t.Run("w/ http config", func(t *testing.T) {
			f, err := os.Open("protocol/http/testdata/http.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			var mocks []protocol.Mock
			if err := yaml.NewDecoder(f).Decode(&mocks); err != nil {
				t.Fatal(err)
			}
			srv, err := NewServer(
				&ServerConfig{
					Protocols: map[string]yamlutil.RawMessage{
						"http": yamlutil.RawMessage("port: 8000"),
					},
					Mocks: mocks,
				},
				logger.NewNopLogger(),
			)
			if err != nil {
				t.Fatalf("failed to create server: %s", err)
			}
			ch := make(chan error)
			go func() {
				ch <- srv.Start(context.Background())
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := srv.Wait(ctx); err != nil {
				t.Errorf("failed to wait: %s", err)
			}
			resp, err := http.Get("http://localhost:8000")
			if err != nil {
				t.Errorf("failed to request: %s", err)
			} else {
				if got, expect := resp.StatusCode, 200; got != expect {
					t.Errorf("expect %d but got %d", expect, got)
				}
			}
			defer resp.Body.Close()
			if err := srv.Stop(ctx); err != nil {
				t.Errorf("failed to stop: %s", err)
			}
			if err := <-ch; err != nil {
				t.Errorf("failed to start: %s", err)
			}
		})
	})
	t.Run("failure", func(t *testing.T) {
		t.Run("config is nil", func(t *testing.T) {
			if _, err := NewServer(nil, logger.NewNopLogger()); err == nil {
				t.Fatal("no error")
			}
		})
	})
}

func TestServer_Success(t *testing.T) {
	srv := &Server{
		servers: map[string]protocol.Server{
			"1": &mockServer{
				start: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
			},
			"2": &mockServer{
				start: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan error)
	go func() {
		ch <- srv.Start(ctx)
	}()
	if err := srv.Wait(context.Background()); err != nil {
		t.Fatalf("failed to wait: %s", err)
	}
	cancel()
	if err := <-ch; err != nil {
		t.Fatalf("failed to start: %s", err)
	}
	addrs, err := srv.Addrs()
	if err != nil {
		t.Fatalf("failed to get addresses: %s", err)
	}
	for name, s := range srv.servers {
		ms, ok := s.(*mockServer)
		if !ok {
			t.Fatalf("unexpected type %T", s)
		}
		if !ms.startCalled {
			t.Errorf("%s server: Start() is not called", name)
		}
		if !ms.stopCalled {
			t.Errorf("%s server: Stop() is not called", name)
		}
		if !ms.waitCalled {
			t.Errorf("%s server: Wait() is not called", name)
		}
		if !ms.addrCalled {
			t.Errorf("%s server: Addr() is not called", name)
		}
		if _, ok := addrs[name]; !ok {
			t.Errorf("%s no address", name)
		}
	}
}

//nolint:cyclop
func TestServer_Failure(t *testing.T) {
	t.Run("Start() failed", func(t *testing.T) {
		expect := errors.New("failed")
		srv := &Server{
			servers: map[string]protocol.Server{
				"ok": &mockServer{
					start: func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					},
				},
				"error": &mockServer{
					start: func(_ context.Context) error {
						time.Sleep(10 * time.Millisecond)
						return expect
					},
				},
			},
		}
		if err := srv.Start(context.Background()); !errors.Is(err, expect) {
			t.Fatalf("expect error %q but got %q", expect, err)
		}
		for name, s := range srv.servers {
			ms, ok := s.(*mockServer)
			if !ok {
				t.Fatalf("unexpected type %T", s)
			}
			if !ms.startCalled {
				t.Errorf("%s server: Start() is not called", name)
			}
			if !ms.stopCalled {
				t.Errorf("%s server: Stop() is not called", name)
			}
		}
	})
	t.Run("Wait() failed", func(t *testing.T) {
		expect := errors.New("failed")
		srv := &Server{
			servers: map[string]protocol.Server{
				"ok": &mockServer{
					start: func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					},
				},
				"error": &mockServer{
					start: func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					},
					wait: func(_ context.Context) error {
						return expect
					},
				},
			},
		}
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan error)
		go func() {
			ch <- srv.Start(ctx)
		}()
		if err := srv.Wait(context.Background()); !errors.Is(err, expect) {
			t.Fatalf("expect error %q but got %q", expect, err)
		}
		cancel()
		if err := <-ch; err != nil {
			t.Fatalf("failed to start: %s", err)
		}
		for name, s := range srv.servers {
			ms, ok := s.(*mockServer)
			if !ok {
				t.Fatalf("unexpected type %T", s)
			}
			if !ms.startCalled {
				t.Errorf("%s server: Start() is not called", name)
			}
			if !ms.stopCalled {
				t.Errorf("%s server: Stop() is not called", name)
			}
			if !ms.waitCalled {
				t.Errorf("%s server: Wait() is not called", name)
			}
		}
	})
	t.Run("Stop() failed", func(t *testing.T) {
		stopErr := errors.New("failed")
		srv := &Server{
			servers: map[string]protocol.Server{
				"1": &mockServer{
					start: func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					},
					stop: func(_ context.Context) error {
						return stopErr
					},
				},
				"2": &mockServer{
					start: func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					},
					stop: func(_ context.Context) error {
						time.Sleep(10 * time.Millisecond)
						return stopErr
					},
				},
			},
		}
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan error)
		go func() {
			ch <- srv.Start(ctx)
		}()
		if err := srv.Wait(context.Background()); err != nil {
			t.Fatalf("failed to wait: %s", err)
		}
		cancel()
		if err := <-ch; err != nil {
			t.Fatalf("failed to start: %s", err)
		}
		if err := srv.Stop(context.Background()); err == nil {
			t.Fatal("no error")
		} else {
			expect := `2 errors occurred:
	* failed to stop 1 server: failed
	* failed to stop 2 server: failed

`
			if got := err.Error(); got != expect {
				t.Fatalf("expect error %q but got %q", expect, got)
			}
		}
		for name, s := range srv.servers {
			ms, ok := s.(*mockServer)
			if !ok {
				t.Fatalf("unexpected type %T", s)
			}
			if !ms.startCalled {
				t.Errorf("%s server: Start() is not called", name)
			}
			if !ms.stopCalled {
				t.Errorf("%s server: Stop() is not called", name)
			}
			if !ms.waitCalled {
				t.Errorf("%s server: Wait() is not called", name)
			}
		}
	})
	t.Run("Addr() failed", func(t *testing.T) {
		expect := errors.New("failed")
		srv := &Server{
			servers: map[string]protocol.Server{
				"ok": &mockServer{},
				"error": &mockServer{
					addr: func() (string, error) {
						return "", expect
					},
				},
			},
		}
		if _, err := srv.Addrs(); !errors.Is(err, expect) {
			t.Fatalf("expect error %q but got %q", expect, err)
		}
	})
	t.Run("mocks remain error", func(t *testing.T) {
		srv, err := NewServer(&ServerConfig{
			Mocks: []protocol.Mock{{}},
		}, logger.NewNopLogger())
		if err != nil {
			t.Fatal(err)
		}
		ch := make(chan error)
		go func() {
			ch <- srv.Start(context.Background())
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := srv.Wait(ctx); err != nil {
			t.Errorf("failed to wait: %s", err)
		}
		if err := srv.Stop(ctx); err == nil {
			t.Error("no error")
		} else {
			if got, expect := err.Error(), "last 1 mocks remain"; got != expect {
				t.Errorf("expect %q but got %q", expect, got)
			}
		}
		if err := <-ch; err != nil {
			t.Errorf("failed to start: %s", err)
		}
	})
}

type mockServer struct {
	start       func(ctx context.Context) error
	startCalled bool
	wait        func(ctx context.Context) error
	waitCalled  bool
	stop        func(ctx context.Context) error
	stopCalled  bool
	addr        func() (string, error)
	addrCalled  bool
}

func (s *mockServer) Start(ctx context.Context) error {
	s.startCalled = true
	if s.start != nil {
		return s.start(ctx)
	}
	return nil
}

func (s *mockServer) Wait(ctx context.Context) error {
	s.waitCalled = true
	if s.wait != nil {
		return s.wait(ctx)
	}
	return nil
}

func (s *mockServer) Stop(ctx context.Context) error {
	s.stopCalled = true
	if s.stop != nil {
		return s.stop(ctx)
	}
	return nil
}

func (s *mockServer) Addr() (string, error) {
	s.addrCalled = true
	if s.addr != nil {
		return s.addr()
	}
	return "", nil
}
