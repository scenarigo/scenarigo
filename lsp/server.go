package lsp

import (
	"context"
	"sync"

	"github.com/go-language-server/jsonrpc2"
	"github.com/go-language-server/protocol"

	// "github.com/zoncoen/scenarigo/lsp/source"
	"go.uber.org/zap"
)

var _ protocol.ServerInterface = (*Server)(nil)

type serverState int

const (
	serverCreated = serverState(iota)
	serverInitializing
	serverInitialized
	serverShutDown
)

type Server struct {
	Conn *jsonrpc2.Conn
	// session source.Session

	stateMu sync.Mutex
	state   serverState

	pendingFolders []protocol.WorkspaceFolder

	logger *zap.Logger
}

func NewServer(ctx context.Context, stream jsonrpc2.Stream, opts ...Option) (*Server, error) {
	srv := &Server{
		// session: source.NewSession(),
	}
	srv.Conn, _ = protocol.NewServer(ctx, srv, stream, zap.NewNop())
	for _, opt := range opts {
		opt(srv)
	}
	return srv, nil
}

type Option func(*Server)

func WithLogger(logger *zap.Logger) Option {
	return func(s *Server) {
		s.logger = logger.Named("server")
	}
}

func (s *Server) Run(ctx context.Context) error {
	return s.Conn.Run(ctx)
}

func (s *Server) Initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	s.logger.Info("initialize")
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{},
	}, nil
	// return s.initialize(ctx, params)
}

func (s *Server) Initialized(ctx context.Context, params *protocol.InitializedParams) (err error) {
	return notImplemented("Initialized")
	// return s.initialized(ctx, params)
}

func (s *Server) Shutdown(ctx context.Context) (err error) {
	return notImplemented("Shutdown")
}

func (s *Server) Exit(ctx context.Context) (err error) {
	return notImplemented("Exit")
}

func (s *Server) CodeAction(ctx context.Context, params *protocol.CodeActionParams) (result []protocol.CodeAction, err error) {
	return nil, notImplemented("CodeAction")
}

func (s *Server) CodeLens(ctx context.Context, params *protocol.CodeLensParams) (result []protocol.CodeLens, err error) {
	return nil, notImplemented("CodeLens")
}

func (s *Server) CodeLensResolve(ctx context.Context, params *protocol.CodeLens) (result *protocol.CodeLens, err error) {
	return nil, notImplemented("CodeLensResolve")
}

func (s *Server) ColorPresentation(ctx context.Context, params *protocol.ColorPresentationParams) (result []protocol.ColorPresentation, err error) {
	return nil, notImplemented("ColorPresentation")
}

func (s *Server) Completion(ctx context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	s.logger.Info("completion", zap.Any("req", params))
	return nil, notImplemented("Completion")
	// return s.completion(ctx, params)
}

func (s *Server) CompletionResolve(ctx context.Context, params *protocol.CompletionItem) (result *protocol.CompletionItem, err error) {
	return nil, notImplemented("CompletionResolve")
}

func (s *Server) Declaration(ctx context.Context, params *protocol.TextDocumentPositionParams) (result []protocol.Location, err error) {
	return nil, notImplemented("Declaration")
}

func (s *Server) Definition(ctx context.Context, params *protocol.TextDocumentPositionParams) (result []protocol.Location, err error) {
	return nil, notImplemented("Definition")
}

func (s *Server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) (err error) {
	return notImplemented("DidChange")
	// return s.didChange(ctx, params)
}

func (s *Server) DidChangeConfiguration(ctx context.Context, params *protocol.DidChangeConfigurationParams) (err error) {
	return notImplemented("DidChangeConfiguration")
}

func (s *Server) DidChangeWatchedFiles(ctx context.Context, params *protocol.DidChangeWatchedFilesParams) (err error) {
	return notImplemented("DidChangeWatchedFiles")
}

func (s *Server) DidChangeWorkspaceFolders(ctx context.Context, params *protocol.DidChangeWorkspaceFoldersParams) (err error) {
	return notImplemented("DidChangeWorkspaceFolders")
}

func (s *Server) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) (err error) {
	return notImplemented("DidClose")
}

func (s *Server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) (err error) {
	return notImplemented("DidOpen")
	// return s.didOpen(ctx, params)
}

func (s *Server) DidSave(ctx context.Context, params *protocol.DidSaveTextDocumentParams) (err error) {
	return notImplemented("DidSave")
}

func (s *Server) DocumentColor(ctx context.Context, params *protocol.DocumentColorParams) (result []protocol.ColorInformation, err error) {
	return nil, notImplemented("DocumentColor")
}

func (s *Server) DocumentHighlight(ctx context.Context, params *protocol.TextDocumentPositionParams) (result []protocol.DocumentHighlight, err error) {
	return nil, notImplemented("DocumentHighlight")
}

func (s *Server) DocumentLink(ctx context.Context, params *protocol.DocumentLinkParams) (result []protocol.DocumentLink, err error) {
	return nil, notImplemented("DocumentLink")
}

func (s *Server) DocumentLinkResolve(ctx context.Context, params *protocol.DocumentLink) (result *protocol.DocumentLink, err error) {
	return nil, notImplemented("DocumentLinkResolve")
}

func (s *Server) DocumentSymbol(ctx context.Context, params *protocol.DocumentSymbolParams) (result []protocol.DocumentSymbol, err error) {
	return nil, notImplemented("DocumentSymbol")
}

func (s *Server) ExecuteCommand(ctx context.Context, params *protocol.ExecuteCommandParams) (result interface{}, err error) {
	return nil, notImplemented("ExecuteCommand")
}

func (s *Server) FoldingRanges(ctx context.Context, params *protocol.FoldingRangeParams) (result []protocol.FoldingRange, err error) {
	return nil, notImplemented("FoldingRanges")
}

func (s *Server) Formatting(ctx context.Context, params *protocol.DocumentFormattingParams) (result []protocol.TextEdit, err error) {
	return nil, notImplemented("Formatting")
}

func (s *Server) Hover(ctx context.Context, params *protocol.TextDocumentPositionParams) (result *protocol.Hover, err error) {
	return nil, notImplemented("Hover")
}

func (s *Server) Implementation(ctx context.Context, params *protocol.TextDocumentPositionParams) (result []protocol.Location, err error) {
	return nil, notImplemented("Implementation")
}

func (s *Server) OnTypeFormatting(ctx context.Context, params *protocol.DocumentOnTypeFormattingParams) (result []protocol.TextEdit, err error) {
	return nil, notImplemented("OnTypeFormatting")
}

func (s *Server) PrepareRename(ctx context.Context, params *protocol.TextDocumentPositionParams) (result *protocol.Range, err error) {
	return nil, notImplemented("PrepareRename")
}

func (s *Server) RangeFormatting(ctx context.Context, params *protocol.DocumentRangeFormattingParams) (result []protocol.TextEdit, err error) {
	return nil, notImplemented("RangeFormatting")
}

func (s *Server) References(ctx context.Context, params *protocol.ReferenceParams) (result []protocol.Location, err error) {
	return nil, notImplemented("References")
}

func (s *Server) Rename(ctx context.Context, params *protocol.RenameParams) (result *protocol.WorkspaceEdit, err error) {
	return nil, notImplemented("Rename")
}

func (s *Server) SignatureHelp(ctx context.Context, params *protocol.TextDocumentPositionParams) (result *protocol.SignatureHelp, err error) {
	return nil, notImplemented("SignatureHelp")
}

func (s *Server) Symbols(ctx context.Context, params *protocol.WorkspaceSymbolParams) (result []protocol.SymbolInformation, err error) {
	return nil, notImplemented("Symbols")
}

func (s *Server) TypeDefinition(ctx context.Context, params *protocol.TextDocumentPositionParams) (result []protocol.Location, err error) {
	return nil, notImplemented("TypeDefinition")
}

func (s *Server) WillSave(ctx context.Context, params *protocol.WillSaveTextDocumentParams) (err error) {
	return notImplemented("WillSave")
}

func (s *Server) WillSaveWaitUntil(ctx context.Context, params *protocol.WillSaveTextDocumentParams) (result []protocol.TextEdit, err error) {
	return nil, notImplemented("WillSaveWaitUntil")
}

func notImplemented(method string) *jsonrpc2.Error {
	return jsonrpc2.Errorf(jsonrpc2.MethodNotFound, "method %q not yet implemented", method)
}
