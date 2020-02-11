package lsp

import (
	"context"

	"github.com/pkg/errors"
)

func (s *Server) addView(ctx context.Context, name string, uri string) error {
	s.stateMu.Lock()
	state := s.state
	s.stateMu.Unlock()

	if state < serverInitialized {
		return errors.Errorf("addView called before server initialized")
	}

	s.session.NewView(ctx, name, uri)

	return nil
}
