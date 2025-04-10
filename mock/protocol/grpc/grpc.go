package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/scenarigo/scenarigo/logger"
	"github.com/scenarigo/scenarigo/mock/protocol"
	"github.com/scenarigo/scenarigo/protocol/grpc/proto"
)

var waitInterval = 100 * time.Millisecond

// Register registers grpc protocol.
func Register() {
	protocol.Register(&GRPC{})
}

// GRPC is a protocol type for the mock.
type GRPC struct{}

// Name implements protocol.Protocol interface.
func (_ GRPC) Name() string { return "grpc" } //nolint:revive

// UnmarshalConfig implements protocol.Protocol interface.
func (_ GRPC) UnmarshalConfig(b []byte) (any, error) { //nolint:revive
	var config ServerConfig
	if err := yaml.Unmarshal(b, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// NewServer implements protocol.Protocol interface.
func (_ *GRPC) NewServer(iter *protocol.MockIterator, l logger.Logger, config any) (protocol.Server, error) { //nolint:revive
	if iter == nil {
		return nil, errors.New("mock iterator is nil")
	}
	cfg, ok := config.(*ServerConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config %T", config)
	}
	srv := &server{
		iter: iter,
	}
	if cfg != nil {
		srv.config = *cfg
		comp := proto.NewCompiler(cfg.Proto.Imports)
		fds, err := comp.Compile(context.Background(), cfg.Proto.Files)
		if err != nil {
			return nil, fmt.Errorf("failed to compile proto: %w", err)
		}
		srv.resolver = fds
	}
	return srv, nil
}

// ServerConfig represents a server configuration.
type ServerConfig struct {
	Port  int         `yaml:"port,omitempty"`
	Proto ProtoConfig `yaml:"proto,omitempty"`
}

// ProtoConfig represents a proto configuration.
type ProtoConfig struct {
	Imports []string `yaml:"imports,omitempty"`
	Files   []string `yaml:"files,omitempty"`
}

type server struct {
	m        sync.Mutex
	config   ServerConfig
	iter     *protocol.MockIterator
	resolver proto.ServiceDescriptorResolver
	addr     string
	srv      *grpc.Server
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
	s.addr = ln.Addr().String()
	s.srv = grpc.NewServer()
	healthpb.RegisterHealthServer(s.srv, &healthServer{})
	names, err := s.resolver.ListServices()
	if err != nil {
		return nil, fmt.Errorf("failed to get service descriptor: %w", err)
	}
	for _, name := range names {
		sd, err := s.resolver.ResolveService(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get service descriptor: %w", err)
		}
		s.srv.RegisterService(s.convertToServicDesc(sd), nil)
	}
	return func() error {
		if err := s.srv.Serve(ln); err != nil {
			if !errors.Is(err, grpc.ErrServerStopped) {
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
	var once sync.Once
	var client healthpb.HealthClient
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.m.Lock()
		srv := s.srv
		s.m.Unlock()
		if srv != nil {
			var err error
			once.Do(func() {
				c, cErr := grpc.NewClient(s.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if cErr != nil {
					err = fmt.Errorf("failed to connect server: %w", err)
					return
				}
				client = healthpb.NewHealthClient(c)
			})
			if err != nil {
				return err
			}
			resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{
				Service: "grpc.health.v1",
			})
			if err == nil {
				if resp.GetStatus() == healthpb.HealthCheckResponse_SERVING {
					return nil
				}
			}
		}
		time.Sleep(waitInterval)
	}
}

// Stop implements protocol.Server interface.
func (s *server) Stop(ctx context.Context) error {
	s.m.Lock()
	defer s.m.Unlock()
	if s.srv == nil {
		return protocol.ErrServerClosed
	}
	s.addr = ""
	srv := s.srv
	s.srv = nil
	srv.GracefulStop() // GracefulStop() calls s.ln.Close()
	return nil
}

// Addr implements protocol.Server interface.
func (s *server) Addr() (string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	if s.srv == nil {
		return "", protocol.ErrServerClosed
	}
	return s.addr, nil
}
