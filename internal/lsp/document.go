package lsp

import (
	"sync"

	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
)

// documentStore manages open text documents and their parsed state.
type documentStore struct {
	mu   sync.RWMutex
	docs map[string]*document
}

type document struct {
	URI     string
	Version int
	Text    string
	Parsed  *yamlutil.Document // latest successful parse
}

func newDocumentStore() *documentStore {
	return &documentStore{
		docs: make(map[string]*document),
	}
}

func (s *documentStore) Open(uri string, version int, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc := &document{
		URI:     uri,
		Version: version,
		Text:    text,
	}
	doc.Parsed = yamlutil.Parse(text)
	s.docs[uri] = doc
}

func (s *documentStore) Update(uri string, version int, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.docs[uri]
	if !ok {
		doc = &document{URI: uri}
		s.docs[uri] = doc
	}
	doc.Version = version
	doc.Text = text
	if parsed := yamlutil.Parse(text); parsed != nil {
		doc.Parsed = parsed
	}
	// If parse failed, keep the previous Parsed (cache strategy).
}

func (s *documentStore) Close(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
}

func (s *documentStore) Get(uri string) *document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.docs[uri]
}
