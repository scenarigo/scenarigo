package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/scenarigo/scenarigo/plugin"
)

func init() {
	plugin.RegisterSetup(startServer)
}

var ServerAddr string

var (
	mu    sync.Mutex
	items = map[string]Item{}
)

func startServer(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		ctx.Reporter().Fatalf("failed to start server: %s", err)
	}
	ServerAddr = ln.Addr().String()

	m := http.NewServeMux()
	m.Handle("/items", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			createItem(w, r)
			return
		case http.MethodGet:
			if names, ok := r.URL.Query()["name"]; ok {
				getItemByName(w, names[0])
				return
			} else {
				getItems(w)
				return
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}))
	m.Handle("/items/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getItemByID(w, strings.TrimPrefix(r.URL.Path, "/items/"))
			return
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
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

func createItem(w http.ResponseWriter, r *http.Request) {
	var item Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}
	item.ID = uuid.NewString()

	mu.Lock()
	defer mu.Unlock()
	items[item.ID] = item
	writeJSON(w, item)
	return
}

func getItems(w http.ResponseWriter) {
	mu.Lock()
	defer mu.Unlock()
	itemList := []Item{}
	for _, item := range items {
		item := item
		itemList = append(itemList, item)
	}
	writeJSON(w, itemList)
	return
}

func getItemByID(w http.ResponseWriter, id string) {
	mu.Lock()
	defer mu.Unlock()
	if item, ok := items[id]; ok {
		writeJSON(w, item)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	return
}

func getItemByName(w http.ResponseWriter, name string) {
	mu.Lock()
	defer mu.Unlock()
	for _, item := range items {
		item := item
		if item.Name == name {
			writeJSON(w, item)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	return
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

type Item struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
}
