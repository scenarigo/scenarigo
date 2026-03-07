package lsp

import "encoding/json"

// JSON-RPC 2.0 message types.

type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  any              `json:"result,omitempty"`
	Error   *ResponseError   `json:"error,omitempty"`
}

type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// LSP protocol types (subset needed for MVP).

type InitializeParams struct {
	RootURI string `json:"rootUri,omitempty"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

type ServerCapabilities struct {
	TextDocumentSync   int                `json:"textDocumentSync"`
	CompletionProvider *CompletionOptions `json:"completionProvider,omitempty"`
	HoverProvider          bool               `json:"hoverProvider,omitempty"`
	DefinitionProvider     bool               `json:"definitionProvider,omitempty"`
	DocumentSymbolProvider bool               `json:"documentSymbolProvider,omitempty"`
	CodeActionProvider     bool               `json:"codeActionProvider,omitempty"`
}

type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type VersionedTextDocumentIdentifier struct {
	TextDocumentIdentifier
	Version int `json:"version"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type Position struct {
	Line      int `json:"line"`      // 0-based
	Character int `json:"character"` // 0-based
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type CompletionItem struct {
	Label         string  `json:"label"`
	Kind          int     `json:"kind,omitempty"`
	Detail        string  `json:"detail,omitempty"`
	Documentation string  `json:"documentation,omitempty"`
	InsertText    string  `json:"insertText,omitempty"`
	SortText      string  `json:"sortText,omitempty"`
}

// CompletionItemKind values.
const (
	CompletionItemKindFunction = 3
	CompletionItemKindField    = 5
	CompletionItemKindVariable = 6
	CompletionItemKindModule   = 9
	CompletionItemKindProperty = 10
	CompletionItemKindValue    = 12
	CompletionItemKindKeyword  = 14
	CompletionItemKindFile     = 17
	CompletionItemKindFolder   = 19
)

type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
}

type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Diagnostic types.

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Message  string `json:"message"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

const (
	DiagnosticSeverityError       = 1
	DiagnosticSeverityWarning     = 2
	DiagnosticSeverityInformation = 3
)

// CodeAction types.

type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type CodeAction struct {
	Title       string        `json:"title"`
	Kind        string        `json:"kind,omitempty"`
	Diagnostics []Diagnostic  `json:"diagnostics,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
}

type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

const CodeActionKindQuickFix = "quickfix"

// DocumentSymbol types.

type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolKind values.
const (
	SymbolKindFile      = 1
	SymbolKindModule    = 2
	SymbolKindNamespace = 3
	SymbolKindPackage   = 4
	SymbolKindClass     = 5
	SymbolKindMethod    = 6
	SymbolKindProperty  = 7
	SymbolKindField     = 8
	SymbolKindVariable  = 13
	SymbolKindConstant  = 14
	SymbolKindString    = 15
	SymbolKindNumber    = 16
	SymbolKindBoolean   = 17
	SymbolKindArray     = 18
	SymbolKindObject    = 19
	SymbolKindKey       = 20
)

// Definition types.

type DefinitionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}
