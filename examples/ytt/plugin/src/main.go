package main

import (
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/scenarigo/scenarigo/plugin"
)

func init() {
	plugin.RegisterSetup(startServer)
}

var ServerAddr string

func startServer(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		ctx.Reporter().Fatalf("failed to start server: %s", err)
	}
	ServerAddr = ln.Addr().String()

	m := http.NewServeMux()
	m.Handle("/echo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Add("Content-Type", r.Header.Get("Content-Type"))
		io.Copy(w, r.Body)
	}))
	s := http.Server{
		Handler: m,
	}
	go func() {
		if err := s.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			ctx.Reporter().Errorf("failed to start server: %s", err)
		}
	}()

	return ctx, func(ctx *plugin.Context) {
		if err := s.Close(); err != nil {
			ctx.Reporter().Errorf("failed to close server: %s", err)
		}
	}
}
