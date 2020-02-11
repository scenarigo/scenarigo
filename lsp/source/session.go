package source

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-language-server/uri"
)

var (
	sessionIndex, viewIndex int64
)

// Session represents a single connection from a client.
type Session interface {
	// NewView creates a new View and returns it.
	NewView(ctx context.Context, name, folder string) View

	// ViewOf returns a view corresponding to the given URI.
	ViewOf(uri uri.URI) View

	// DidOpen is invoked each time a file is opened in the editor.
	DidOpen(ctx context.Context, uri uri.URI, text []byte)

	DidChange(ctx context.Context, uri uri.URI, text []byte)
}

type session struct {
	id int64

	viewMu  sync.Mutex
	views   []View
	viewMap map[string]View

	overlayMu sync.Mutex
	overlays  map[uri.URI]*overlay

	openFiles sync.Map
}

func NewSession() Session {
	index := atomic.AddInt64(&sessionIndex, 1)
	return &session{
		id:       index,
		viewMap:  make(map[string]View),
		overlays: make(map[uri.URI]*overlay),
	}
}

func (s *session) NewView(ctx context.Context, name, folder string) View {
	index := atomic.AddInt64(&viewIndex, 1)
	s.viewMu.Lock()
	defer s.viewMu.Unlock()

	v := &view{
		session: s,
		id:      index,
		name:    name,
		folder:  folder,
	}
	s.views = append(s.views, v)
	// we always need to drop the view map
	s.viewMap = make(map[string]View)
	return v
}

func (s *session) ViewOf(uri uri.URI) View {
	s.viewMu.Lock()
	defer s.viewMu.Unlock()

	// Check if we already know this file.
	if v, found := s.viewMap[string(uri)]; found {
		return v
	}

	// Pick the best view for this file and memoize the result.
	v := s.bestView(uri)
	s.viewMap[string(uri)] = v
	return v
}

// bestView finds the best view to associate a given URI with.
// viewMu must be held when calling this method.
func (s *session) bestView(uri uri.URI) View {
	// we need to find the best view for this file
	var longest View
	for _, view := range s.views {
		if longest != nil && len(longest.Folder()) > len(view.Folder()) {
			continue
		}
		if strings.HasPrefix(string(uri), string(view.Folder())) {
			longest = view
		}
	}
	if longest != nil {
		return longest
	}
	return s.views[0]
}

func (s *session) DidOpen(ctx context.Context, uri uri.URI, text []byte) {
	s.openFiles.Store(uri, true)
	s.openOverlay(ctx, uri, text)
}

func (s *session) DidChange(ctx context.Context, uri uri.URI, text []byte) {
	s.openOverlay(ctx, uri, text)
}

func (s *session) openOverlay(ctx context.Context, uri uri.URI, data []byte) {
	s.overlayMu.Lock()
	defer s.overlayMu.Unlock()
	s.overlays[uri] = &overlay{
		uri:       uri,
		data:      data,
		unchanged: true,
	}

	s.overlays[uri].lines = strings.Split(string(data), "\n")

	// tokens := lexer.Tokenize(string(data))
	// p := &parser.Parser{}
	// doc, err := p.Parse(tokens)
	// if err != nil {
	// 	return
	// }
	// s.overlays[uri].doc = doc
}
