package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/scenarigo/scenarigo/internal/lsp/schema"
	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
)

// Server is the LSP server.
type Server struct {
	reader io.Reader
	writer io.Writer
	logger *log.Logger
	docs   *documentStore
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

// Run starts the LSP server main loop.
func (s *Server) Run() error {
	reader := bufio.NewReader(s.reader)

	for {
		// Read headers until empty line.
		contentLength := -1
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return fmt.Errorf("read header error: %w", err)
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break // End of headers.
			}
			if strings.HasPrefix(line, "Content-Length: ") {
				n, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
				if err != nil {
					return fmt.Errorf("invalid Content-Length: %w", err)
				}
				contentLength = n
			}
		}

		if contentLength < 0 {
			continue
		}

		// Read content body.
		body := make([]byte, contentLength)
		if _, err := io.ReadFull(reader, body); err != nil {
			return fmt.Errorf("read body error: %w", err)
		}

		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			s.logger.Printf("failed to unmarshal request: %v", err)
			continue
		}

		s.handleMessage(&req)
	}
}

func (s *Server) handleMessage(req *Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		// No action needed.
	case "shutdown":
		s.sendResponse(req.ID, nil, nil)
	case "exit":
		os.Exit(0)
	case "textDocument/didOpen":
		s.handleDidOpen(req)
	case "textDocument/didChange":
		s.handleDidChange(req)
	case "textDocument/didClose":
		s.handleDidClose(req)
	case "textDocument/completion":
		s.handleCompletion(req)
	case "textDocument/hover":
		s.handleHover(req)
	default:
		if req.ID != nil {
			// Unknown request - return method not found.
			s.sendResponse(req.ID, nil, &ResponseError{
				Code:    -32601,
				Message: "method not found: " + req.Method,
			})
		}
	}
}

func (s *Server) handleInitialize(req *Request) {
	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: 1, // Full sync.
			CompletionProvider: &CompletionOptions{
				TriggerCharacters: []string{":", " ", "\n"},
			},
			HoverProvider: true,
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
	s.docs.Open(params.TextDocument.URI, params.TextDocument.Version, params.TextDocument.Text)
	s.publishDiagnostics(params.TextDocument.URI)
}

func (s *Server) handleDidChange(req *Request) {
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("didChange unmarshal error: %v", err)
		return
	}
	if len(params.ContentChanges) > 0 {
		s.docs.Update(params.TextDocument.URI, params.TextDocument.Version, params.ContentChanges[len(params.ContentChanges)-1].Text)
	}
	s.publishDiagnostics(params.TextDocument.URI)
}

func (s *Server) handleDidClose(req *Request) {
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("didClose unmarshal error: %v", err)
		return
	}
	s.docs.Close(params.TextDocument.URI)
	// Clear diagnostics.
	s.sendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []Diagnostic{},
	})
}

func (s *Server) handleCompletion(req *Request) {
	var params CompletionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("completion unmarshal error: %v", err)
		s.sendResponse(req.ID, nil, nil)
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(req.ID, CompletionList{}, nil)
		return
	}

	items := s.complete(doc, params.Position)
	s.sendResponse(req.ID, CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil)
}

func (s *Server) handleHover(req *Request) {
	var params HoverParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("hover unmarshal error: %v", err)
		s.sendResponse(req.ID, nil, nil)
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}

	hover := s.hover(doc, params.Position)
	if hover == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}
	s.sendResponse(req.ID, hover, nil)
}

func (s *Server) complete(doc *document, pos Position) []CompletionItem {
	sch := schema.DetectSchemaType(doc.Text)

	// Use the parsed document for cursor context, fall back to text-based analysis.
	var ctx *yamlutil.CursorContext
	if doc.Parsed != nil {
		ctx = doc.Parsed.GetCursorContext(pos.Line, pos.Character)
	}
	if ctx == nil {
		return nil
	}

	switch ctx.Type {
	case yamlutil.CursorContextKey:
		return s.completeKeys(sch, ctx)
	case yamlutil.CursorContextValue:
		return s.completeValues(sch, ctx)
	default:
		return nil
	}
}

func (s *Server) completeKeys(sch *schema.Schema, ctx *yamlutil.CursorContext) []CompletionItem {
	fields := sch.ChildFields(ctx.Path, ctx.SiblingValues)
	if fields == nil {
		return nil
	}

	// Filter out keys that already exist at this level.
	existing := make(map[string]bool)
	for _, k := range ctx.ParentKeys {
		existing[k] = true
	}

	var items []CompletionItem
	for i, f := range fields {
		if existing[f.Name] {
			continue
		}
		if ctx.PartialKey != "" && !strings.HasPrefix(f.Name, ctx.PartialKey) {
			continue
		}

		insertText := f.Name + ": "
		if f.Type == schema.FieldTypeObject {
			insertText = f.Name + ":"
		} else if f.Type == schema.FieldTypeArray {
			insertText = f.Name + ":"
		}

		items = append(items, CompletionItem{
			Label:         f.Name,
			Kind:          CompletionItemKindField,
			Detail:        f.Type.String(),
			Documentation: f.Description,
			InsertText:    insertText,
			SortText:      fmt.Sprintf("%03d_%s", i, f.Name),
		})
	}
	return items
}

func (s *Server) completeValues(sch *schema.Schema, ctx *yamlutil.CursorContext) []CompletionItem {
	if len(ctx.Path) == 0 {
		return nil
	}

	field := sch.FindField(ctx.Path)
	if field == nil {
		return nil
	}

	// Enum values.
	if len(field.EnumValues) > 0 {
		var items []CompletionItem
		for _, v := range field.EnumValues {
			if ctx.PartialValue != "" && !strings.HasPrefix(v, ctx.PartialValue) {
				continue
			}
			items = append(items, CompletionItem{
				Label:  v,
				Kind:   CompletionItemKindValue,
				Detail: field.Description,
			})
		}
		return items
	}

	// Bool values.
	if field.Type == schema.FieldTypeBool {
		return []CompletionItem{
			{Label: "true", Kind: CompletionItemKindValue},
			{Label: "false", Kind: CompletionItemKindValue},
		}
	}

	return nil
}

func (s *Server) hover(doc *document, pos Position) *Hover {
	sch := schema.DetectSchemaType(doc.Text)
	if doc.Parsed == nil {
		return nil
	}

	// Find node at position (convert from 0-based to 1-based).
	nodePath := doc.Parsed.FindNodeAtPosition(pos.Line+1, pos.Character+1)
	if nodePath == nil || len(nodePath.Keys) == 0 {
		return nil
	}

	field := sch.FindField(nodePath.Keys)
	if field == nil {
		return nil
	}

	content := fmt.Sprintf("**%s** (`%s`)\n\n%s", field.Name, field.Type, field.Description)
	if len(field.EnumValues) > 0 {
		content += fmt.Sprintf("\n\nAllowed values: `%s`", strings.Join(field.EnumValues, "`, `"))
	}

	return &Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: content,
		},
	}
}

func (s *Server) publishDiagnostics(uri string) {
	doc := s.docs.Get(uri)
	if doc == nil {
		return
	}

	var diagnostics []Diagnostic

	// Check if YAML parsing failed.
	if doc.Parsed == nil {
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 0},
			},
			Severity: DiagnosticSeverityError,
			Message:  "Invalid YAML syntax",
		})
	}

	s.sendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	})
}

func (s *Server) sendResponse(id *json.RawMessage, result any, respErr *ResponseError) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   respErr,
	}
	s.writeMessage(resp)
}

func (s *Server) sendNotification(method string, params any) {
	p, _ := json.Marshal(params)
	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  p,
	}
	s.writeMessage(notif)
}

func (s *Server) writeMessage(msg any) {
	body, err := json.Marshal(msg)
	if err != nil {
		s.logger.Printf("marshal error: %v", err)
		return
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := fmt.Fprint(s.writer, header); err != nil {
		s.logger.Printf("write header error: %v", err)
		return
	}
	if _, err := s.writer.Write(body); err != nil {
		s.logger.Printf("write body error: %v", err)
	}
}
