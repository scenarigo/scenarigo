package lsp

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
)

// Server is the LSP server.
type Server struct {
	reader            io.Reader
	writer            io.Writer
	logger            *log.Logger
	docs              *documentStore
	rootURI           string
	encoding          positionEncoding // unit of Position.Character on the wire, see position.go
	writeMu           sync.Mutex       // serializes writeMessage
	shutdownRequested bool
}

// NewServer creates a new LSP server that communicates over stdio.
func NewServer() *Server {
	return &Server{
		reader: os.Stdin,
		writer: os.Stdout,
		logger: log.New(os.Stderr, "[scenarigo-lsp] ", log.LstdFlags),
		docs:   newDocumentStore(),
	}
}

func (s *Server) handleMessage(req *Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		// No action needed.
	case "shutdown":
		s.shutdownRequested = true
		s.sendResponse(req.ID, nil, nil)
	case "exit":
		// The exit code is 0 only when shutdown was requested first.
		if s.shutdownRequested {
			os.Exit(0)
		}
		os.Exit(1)
	case "textDocument/didOpen":
		s.handleDidOpen(req)
	case "textDocument/didChange":
		s.handleDidChange(req)
	case "textDocument/didClose":
		s.handleDidClose(req)
	default:
		if req.ID != nil {
			// Unknown request - return method not found.
			s.sendResponse(req.ID, nil, &ResponseError{
				Code:    codeMethodNotFound,
				Message: "method not found: " + req.Method,
			})
		}
	}
}

func (s *Server) handleInitialize(req *Request) {
	// Read rootUri if present.
	var initParams InitializeParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &initParams); err == nil {
			s.rootURI = initParams.RootURI
		}
	}
	var offered []string
	if initParams.Capabilities.General != nil {
		offered = initParams.Capabilities.General.PositionEncodings
	}
	s.encoding = negotiateEncoding(offered)

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			PositionEncoding: string(s.encoding),
			TextDocumentSync: 1, // Full sync.
		},
	}
	s.sendResponse(req.ID, result, nil)
}

func (s *Server) handleDidOpen(req *Request) {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("didOpen unmarshal error: %v", err)
		return
	}
	if hasForeignModeline(params.TextDocument.Text) {
		// File is managed by another YAML language server; skip to save memory.
		return
	}
	s.docs.Open(params.TextDocument.URI, params.TextDocument.Version, params.TextDocument.Text)
}

func (s *Server) handleDidChange(req *Request) {
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("didChange unmarshal error: %v", err)
		return
	}
	if len(params.ContentChanges) > 0 {
		text := params.ContentChanges[len(params.ContentChanges)-1].Text
		if hasForeignModeline(text) {
			// A modeline was added; drop from store to free memory.
			s.docs.Close(params.TextDocument.URI)
			return
		}
		s.docs.Update(params.TextDocument.URI, params.TextDocument.Version, text)
	}
}

func (s *Server) handleDidClose(req *Request) {
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("didClose unmarshal error: %v", err)
		return
	}
	s.docs.Close(params.TextDocument.URI)
}
