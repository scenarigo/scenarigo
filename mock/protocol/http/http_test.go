package http

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/scenarigo/scenarigo/logger"
	"github.com/scenarigo/scenarigo/mock/protocol"
)

func init() {
	Register()
}

func TestHTTP_Server(t *testing.T) {
	tests := map[string]struct {
		filename string
		config   string
		f        func(*testing.T, string)
	}{
		"simple": {
			filename: "testdata/http.yaml",
			f: func(t *testing.T, addr string) {
				t.Helper()
				resp, err := http.Get(fmt.Sprintf("http://%s", addr))
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				if got, expect := resp.StatusCode, http.StatusOK; got != expect {
					t.Errorf("expect %d but got %d", expect, got)
				}
			},
		},
		"specify port": {
			filename: "testdata/http.yaml",
			config:   "port: 8888",
			f: func(t *testing.T, addr string) {
				t.Helper()
				resp, err := http.Get("http://localhost:8888")
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				if got, expect := resp.StatusCode, http.StatusOK; got != expect {
					t.Errorf("expect %d but got %d", expect, got)
				}
			},
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			p := protocol.Get("http")
			if p == nil {
				t.Fatal("failed to get protocol")
			}
			f, err := os.Open(test.filename)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			var mocks []protocol.Mock
			if err := yaml.NewDecoder(f).Decode(&mocks); err != nil {
				t.Fatal(err)
			}
			iter := protocol.NewMockIterator(mocks)
			defer func() {
				if err := iter.Stop(); err != nil {
					t.Errorf("failed to stop mock iterator: %s", err)
				}
			}()

			// unmarshal config
			cfg, err := p.UnmarshalConfig([]byte(test.config))
			if err != nil {
				t.Fatalf("failed to unmarshal config: %s", err)
			}

			// start server
			srv, err := p.NewServer(iter, logger.NewNopLogger(), cfg)
			if err != nil {
				t.Fatalf("failed to create server: %s", err)
			}
			go func() {
				if err := srv.Start(context.Background()); err != nil {
					t.Errorf("failed to start server: %s", err)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := srv.Wait(ctx); err != nil {
				t.Fatalf("failed to start server: %s", err)
			}

			addr, err := srv.Addr()
			if err != nil {
				t.Errorf("failed to get address: %s", err)
			}
			test.f(t, addr)

			// stop server
			ctx, cancel = context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := srv.Stop(ctx); err != nil {
				t.Fatalf("failed to stop server: %s", err)
			}
		})
	}
}

func TestHTTP_Server_Start_Failure(t *testing.T) {
	tests := map[string]struct {
		server *server
		expect string
	}{
		"server already started": {
			server: &server{
				srv: &http.Server{
					ReadHeaderTimeout: time.Second,
				},
			},
			expect: "server already started",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			err := test.server.Start(ctx)
			if err == nil {
				t.Fatal("no error")
			}
			if got, expect := err.Error(), test.expect; got != expect {
				t.Fatalf("expect %q but got %q", expect, got)
			}
		})
	}
}
