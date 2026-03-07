package lsp

import (
	"strings"
	"sync"

	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
)

// hasForeignModeline checks if the YAML text contains a modeline comment
// indicating it is managed by another YAML language server.
// e.g., "# yaml-language-server: $schema=..."
func hasForeignModeline(text string) bool {
	for _, line := range strings.SplitN(text, "\n", 20) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# yaml-language-server:") {
			return true
		}
	}
	return false
}

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
