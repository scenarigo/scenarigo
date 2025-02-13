package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/scenarigo/scenarigo/plugin"
)

func init() {
	plugin.RegisterSetup(startServer)
}

var (
	ServerAddr string
	Nil        interface{} = nil
)

func startServer(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		ctx.Reporter().Fatalf("failed to start server: %s", err)
	}
	ServerAddr = ln.Addr().String()

	m := http.NewServeMux()
	m.Handle("/createUser", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		// creating user here
		resp := &Response{
			Name: req.Name,
			Age:  req.Age,
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
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type Response struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}
