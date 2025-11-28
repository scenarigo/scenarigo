package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

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
		now := time.Now()
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Add("Content-Type", r.Header.Get("Content-Type"))
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		resp := &Response{
			Message:    req.Message,
			RecievedAt: now.Format(time.RFC3339),
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
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

type Request struct {
	Message string `json:"message"`
}

type Response struct {
	Message    string `json:"message"`
	RecievedAt string `json:"recievedAt"`
}
