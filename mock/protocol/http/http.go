package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/scenarigo/scenarigo/logger"
	"github.com/scenarigo/scenarigo/mock/protocol"
)

// Register registers http protocol.
func Register() {
	protocol.Register(&HTTP{})
}

const healthPath = "/_health"

// HTTP is a protocol type for the mock.
type HTTP struct{}

// Name implements protocol.Protocol interface.
func (_ HTTP) Name() string { return "http" } //nolint:revive

// UnmarshalConfig implements protocol.Protocol interface.
func (_ HTTP) UnmarshalConfig(b []byte) (interface{}, error) { //nolint:revive
	var config ServerConfig
	if err := yaml.Unmarshal(b, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// NewServer implements protocol.Protocol interface.
func (_ *HTTP) NewServer(iter *protocol.MockIterator, l logger.Logger, config interface{}) (protocol.Server, error) { //nolint:revive
	if iter == nil {
		return nil, errors.New("mock iterator is nil")
	}
	cfg, ok := config.(*ServerConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config %T", config)
	}
	srv := &server{
		handler: NewHandler(iter, l),
	}
	if cfg != nil {
		srv.config = *cfg
	}
	return srv, nil
}

// ServerConfig represents a server configuration.
type ServerConfig struct {
	Port int `yaml:"port,omitempty"`
}

type server struct {
	m       sync.Mutex
	handler http.Handler
	config  ServerConfig
	srv     *http.Server
}

// Start implements protocol.Server interface.
func (s *server) Start(ctx context.Context) error {
	s.m.Lock()
	serve, err := s.setup()
	if err != nil {
		s.m.Unlock()
		return err
	}
	s.m.Unlock()
	return serve()
}

func (s *server) setup() (func() error, error) {
	if s.srv != nil {
		return nil, errors.New("server already started")
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.config.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}
	s.srv = &http.Server{
		Addr: ln.Addr().String(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == healthPath {
				w.WriteHeader(http.StatusOK)
				return
			}
			s.handler.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return func() error {
		if err := s.srv.Serve(ln); err != nil {
			if err != http.ErrServerClosed {
				return err
			}
		}
		return nil
	}, nil
}

// Wait implements protocol.Server interface.
func (s *server) Wait(ctx context.Context) error {
	ch := make(chan error)
	go func() {
		ch <- s.wait(ctx)
	}()
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-ch:
		return err
	}
}

func (s *server) wait(ctx context.Context) error {
	client := &http.Client{
		Timeout: time.Second,
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.m.Lock()
		srv := s.srv
		s.m.Unlock()
		if srv != nil {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s%s", srv.Addr, healthPath), nil)
			if err != nil {
				return err
			}
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Stop implements protocol.Server interface.
func (s *server) Stop(ctx context.Context) error {
	s.m.Lock()
	defer s.m.Unlock()
	if s.srv == nil {
		return protocol.ErrServerClosed
	}
	srv := s.srv
	s.srv = nil
	return srv.Shutdown(ctx)
}

// Addr implements protocol.Server interface.
func (s *server) Addr() (string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	if s.srv == nil {
		return "", protocol.ErrServerClosed
	}
	return s.srv.Addr, nil
}
