package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/scenarigo/scenarigo/plugin"
)

var server *httptest.Server

func init() {
	plugin.RegisterSetup(setupServer)
}

type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func setupServer(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	mux := http.NewServeMux()

	// Success endpoint - returns item data
	mux.HandleFunc("/api/items/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		item := Item{
			ID:   1,
			Name: "test-item",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(item)
	})

	// Endpoint that returns 404
	mux.HandleFunc("/api/items/999", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "item not found",
		})
	})

	// List endpoint
	mux.HandleFunc("/api/items", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		items := []Item{
			{ID: 1, Name: "test-item"},
			{ID: 2, Name: "another-item"},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(items)
	})

	server = httptest.NewServer(mux)
	ctx.Reporter().Logf("Test server started at %s", server.URL)

	// Return teardown function
	return ctx, func(ctx *plugin.Context) {
		if server != nil {
			server.Close()
			ctx.Reporter().Log("Test server stopped")
		}
	}
}

// ServerAddr returns the test server address
var ServerAddr = func() string {
	if server == nil {
		return ""
	}
	return server.URL
}
