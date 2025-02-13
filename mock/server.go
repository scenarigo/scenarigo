package mock

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/hashicorp/go-multierror"
	"github.com/scenarigo/scenarigo/internal/yamlutil"
	"github.com/scenarigo/scenarigo/logger"
	"github.com/scenarigo/scenarigo/mock/protocol"
	"github.com/scenarigo/scenarigo/mock/protocol/http"
)

func init() {
	http.Register()
}

// NewServer returns a new mock server.
func NewServer(config *ServerConfig, l logger.Logger) (*Server, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}
	iter := protocol.NewMockIterator(config.Mocks)
	protocols := protocol.All()
	servers := map[string]protocol.Server{}
	for name, p := range protocols {
		p := p
		var b []byte
		if config.Protocols != nil {
			if msg, ok := config.Protocols[p.Name()]; ok {
				b = []byte(msg)
			}
		}
		cfg, err := p.UnmarshalConfig(b)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s config: %w", name, err)
		}
		s, err := p.NewServer(iter, l, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create %s server: %w", name, err)
		}
		servers[name] = s
	}
	return &Server{
		iter:    iter,
		servers: servers,
		logger:  l,
	}, nil
}

// Server represents a mock server.
type Server struct {
	iter    *protocol.MockIterator
	servers map[string]protocol.Server
	logger  logger.Logger
}

// ServerConfig represents a mock server configuration.
type ServerConfig struct {
	Mocks     []protocol.Mock                `yaml:"mocks,omitempty"`
	Protocols map[string]yamlutil.RawMessage `yaml:"protocols,omitempty"`
}

func (s *Server) Start(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	for name, s := range s.servers {
		name := name
		s := s
		eg.Go(func() error {
			defer func() {
				_ = s.Stop(context.Background())
			}()
			if err := s.Start(ctx); err != nil {
				return fmt.Errorf("failed to start %s server: %w", name, err)
			}
			return nil
		})
	}
	return eg.Wait()
}

func (s *Server) Wait(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	for name, s := range s.servers {
		name := name
		s := s
		eg.Go(func() error {
			if err := s.Wait(ctx); err != nil {
				return fmt.Errorf("failed to wait %s server: %w", name, err)
			}
			return nil
		})
	}
	return eg.Wait()
}

func (s *Server) Stop(ctx context.Context) error {
	var (
		m    sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)
	for name, s := range s.servers {
		name := name
		s := s
		wg.Add(1)
		go func() {
			if err := s.Stop(ctx); err != nil {
				if !errors.Is(err, protocol.ErrServerClosed) {
					m.Lock()
					defer m.Unlock()
					errs = append(errs, fmt.Errorf("failed to stop %s server: %w", name, err))
				}
			}
			wg.Done()
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		return multierror.Append(nil, errs...)
	}
	if s.iter != nil {
		if err := s.iter.Stop(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) Addrs() (map[string]string, error) {
	addrs := map[string]string{}
	for name, s := range s.servers {
		name := name
		s := s
		addr, err := s.Addr()
		if err != nil {
			return nil, fmt.Errorf("failed to get %s server address: %w", name, err)
		}
		addrs[name] = addr
	}
	return addrs, nil
}
