package lsp

import (
	"context"

	"github.com/go-language-server/protocol"
)

func (s *Server) didOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) (err error) {
	text := []byte(params.TextDocument.Text)
	s.session.DidOpen(ctx, params.TextDocument.URI, text)
	return nil
}

func (s *Server) didChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) (err error) {
	text := []byte(params.ContentChanges[0].Text)
	s.session.DidChange(ctx, params.TextDocument.URI, text)
	return nil
}
