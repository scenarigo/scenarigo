package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/zoncoen/scenarigo/plugin"
)

var (
	ServerAddr  string
	accessToken = "YYYYY"
)

func init() {
	plugin.RegisterSetup(startServer)
}

func startServer(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		ctx.Reporter().Fatalf("failed to start server: %s", err)
	}
	ServerAddr = ln.Addr().String()

	m := http.NewServeMux()
	m.Handle("/oauth/token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.FormValue("client_id") != clientID {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.FormValue("client_secret") != clientSecret {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		b, err := json.Marshal(map[string]string{
			"access_token": accessToken,
			"token_type":   "Bearer",
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		w.Write(b)
	}))
	m.Handle("/users/zoncoen", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", accessToken) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		b, err := json.Marshal(map[string]string{
			"name": "zoncoen",
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		w.Write(b)
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
