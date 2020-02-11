package lsp

import (
	"context"
	"path"

	"github.com/go-language-server/jsonrpc2"
	"github.com/go-language-server/protocol"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func (s *Server) initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	s.logger.With(zap.Any("params", params)).Info("initialize")

	s.stateMu.Lock()
	state := s.state
	s.stateMu.Unlock()
	if state >= serverInitializing {
		return nil, jsonrpc2.Errorf(jsonrpc2.InvalidRequest, "server already initialized")
	}
	s.stateMu.Lock()
	s.state = serverInitializing
	s.stateMu.Unlock()

	s.pendingFolders = params.WorkspaceFolders
	if len(s.pendingFolders) == 0 {
		if params.RootURI == "" {
			return nil, errors.Errorf("single file mode was not supported yet")
		}
		s.pendingFolders = []protocol.WorkspaceFolder{{
			URI:  string(params.RootURI),
			Name: path.Base(string(params.RootURI)),
		}}
	}

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			CompletionProvider: &protocol.CompletionOptions{},
			Workspace: &protocol.ServerCapabilitiesWorkspace{
				WorkspaceFolders: &protocol.ServerCapabilitiesWorkspaceFolders{
					Supported:           true,
					ChangeNotifications: "workspace/didChangeWorkspaceFolders",
				},
			},
		},
	}, nil
}

func (s *Server) initialized(ctx context.Context, params *protocol.InitializedParams) (err error) {
	s.stateMu.Lock()
	s.state = serverInitialized
	s.stateMu.Unlock()

	for _, folder := range s.pendingFolders {
		if err := s.addView(ctx, folder.Name, folder.URI); err != nil {
			return err
		}
	}
	s.pendingFolders = nil

	return nil
}
