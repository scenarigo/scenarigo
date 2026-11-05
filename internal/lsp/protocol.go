package lsp

import "encoding/json"

// JSON-RPC 2.0 message types.

type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

// Response is a successful JSON-RPC response. A successful response must
// carry "result" even when it is null, so the field is never omitted.
type Response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  any              `json:"result"`
	Error   *ResponseError   `json:"error,omitempty"`
}

// errorResponse is a failed JSON-RPC response, which must not carry "result".
type errorResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Error   *ResponseError   `json:"error"`
}

type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC 2.0 error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// LSP protocol types (subset needed for MVP).

type InitializeParams struct {
	RootURI      string             `json:"rootUri,omitempty"`
	Capabilities ClientCapabilities `json:"capabilities"`
}

type ClientCapabilities struct {
	General *GeneralClientCapabilities `json:"general,omitempty"`
}

type GeneralClientCapabilities struct {
	// PositionEncodings lists the encodings of Position.Character the client
	// supports, in order of preference. Absent means utf-16 only.
	PositionEncodings []string `json:"positionEncodings,omitempty"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

type ServerCapabilities struct {
	PositionEncoding   string             `json:"positionEncoding,omitempty"`
	TextDocumentSync   int                `json:"textDocumentSync"`
	CompletionProvider *CompletionOptions `json:"completionProvider,omitempty"`
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
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
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
	Context      *CompletionContext     `json:"context,omitempty"`
}

// CompletionContext tells how completion was triggered.
type CompletionContext struct {
	TriggerKind      int    `json:"triggerKind"`
	TriggerCharacter string `json:"triggerCharacter,omitempty"`
}

// CompletionTriggerKind values.
const (
	CompletionTriggerKindInvoked          = 1
	CompletionTriggerKindTriggerCharacter = 2
)

type Position struct {
	Line      int `json:"line"`      // 0-based
	Character int `json:"character"` // 0-based
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type CompletionItem struct {
	Label         string    `json:"label"`
	Kind          int       `json:"kind,omitempty"`
	Detail        string    `json:"detail,omitempty"`
	Documentation string    `json:"documentation,omitempty"`
	InsertText    string    `json:"insertText,omitempty"`
	TextEdit      *TextEdit `json:"textEdit,omitempty"`
	SortText      string    `json:"sortText,omitempty"`
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
	DiagnosticSeverityHint        = 4
)

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}
