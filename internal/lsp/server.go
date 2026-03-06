package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml/ast"

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
	case "textDocument/definition":
		s.handleDefinition(req)
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
			HoverProvider:      true,
			DefinitionProvider: true,
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

func (s *Server) handleDefinition(req *Request) {
	var params DefinitionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("definition unmarshal error: %v", err)
		s.sendResponse(req.ID, nil, nil)
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}

	loc := s.definition(doc, params)
	if loc == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}
	s.sendResponse(req.ID, loc, nil)
}

func (s *Server) definition(doc *document, params DefinitionParams) *Location {
	if doc.Parsed == nil {
		return nil
	}

	// Get cursor context to determine which field we're on.
	ctx := doc.Parsed.GetCursorContext(params.Position.Line, params.Position.Character)
	if ctx == nil || len(ctx.Path) == 0 {
		return nil
	}

	lastKey := ctx.Path[len(ctx.Path)-1]

	// Handle "include" field: jump to the referenced scenario file.
	// Handle "plugins" values: jump to the plugin source.
	// Handle "scenarios" values: jump to scenario files.
	switch {
	case lastKey == "include":
		// Value is a file path relative to the current document.
		return s.resolveFileLocation(params.TextDocument.URI, ctx.PartialValue)
	case ctx.Type == yamlutil.CursorContextValue:
		// Check if we're in a plugins mapping or scenarios array.
		for _, key := range ctx.Path {
			if key == "plugins" || key == "scenarios" {
				return s.resolveFileLocation(params.TextDocument.URI, ctx.PartialValue)
			}
		}
	}

	return nil
}

func (s *Server) resolveFileLocation(docURI, filePath string) *Location {
	if filePath == "" {
		return nil
	}

	// Convert document URI to directory path.
	docPath := uriToPath(docURI)
	if docPath == "" {
		return nil
	}

	dir := filepath.Dir(docPath)
	resolved := filepath.Join(dir, filePath)

	// Check if file exists.
	if _, err := os.Stat(resolved); err != nil {
		return nil
	}

	return &Location{
		URI: pathToURI(resolved),
		Range: Range{
			Start: Position{Line: 0, Character: 0},
			End:   Position{Line: 0, Character: 0},
		},
	}
}

func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return strings.TrimPrefix(uri, "file://")
	}
	return ""
}

func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "file://" + path
	}
	return "file://" + abs
}

func (s *Server) complete(doc *document, pos Position) []CompletionItem {
	// Check if cursor is inside a template expression {{ }}.
	if tmplExpr, ok := getTemplateContext(doc.Text, pos); ok {
		return s.completeTemplate(tmplExpr)
	}

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

// getTemplateContext checks if the cursor is inside {{ }} and returns the
// partial expression being typed.
func getTemplateContext(text string, pos Position) (string, bool) {
	lines := strings.Split(text, "\n")
	if pos.Line >= len(lines) {
		return "", false
	}
	line := lines[pos.Line]
	if pos.Character > len(line) {
		return "", false
	}
	// Look backwards from cursor for "{{".
	prefix := line[:pos.Character]
	openIdx := strings.LastIndex(prefix, "{{")
	if openIdx < 0 {
		return "", false
	}
	// Check there's no closing "}}" between {{ and cursor.
	between := prefix[openIdx+2:]
	if strings.Contains(between, "}}") {
		return "", false
	}
	return strings.TrimSpace(between), true
}

func (s *Server) completeTemplate(expr string) []CompletionItem {
	// Parse the expression to determine what to complete.
	// "vars" → complete top-level template variables
	// "vars." → complete after dot
	// "" → complete all top-level names

	// Top-level template names available in scenarigo.
	topLevel := []templateCandidate{
		{"vars", "Scenario/step variables", CompletionItemKindVariable},
		{"secrets", "Secret variables", CompletionItemKindVariable},
		{"plugins", "Plugin exports", CompletionItemKindVariable},
		{"request", "Request data (protocol-specific)", CompletionItemKindVariable},
		{"response", "Response data (protocol-specific)", CompletionItemKindVariable},
		{"steps", "Results from previous steps", CompletionItemKindVariable},
		{"env", "Environment variable", CompletionItemKindVariable},
		{"assert", "Assertion functions", CompletionItemKindModule},
		{"size", "Get size of collection", CompletionItemKindFunction},
		{"type", "Get type name", CompletionItemKindFunction},
		{"int", "Convert to int", CompletionItemKindFunction},
		{"uint", "Convert to uint", CompletionItemKindFunction},
		{"float", "Convert to float", CompletionItemKindFunction},
		{"bool", "Convert to bool", CompletionItemKindFunction},
		{"string", "Convert to string", CompletionItemKindFunction},
		{"bytes", "Convert to bytes", CompletionItemKindFunction},
		{"time", "Time type conversion", CompletionItemKindFunction},
		{"duration", "Duration type conversion", CompletionItemKindFunction},
	}

	// If there's a dot, we need to complete after the prefix.
	dotIdx := strings.LastIndex(expr, ".")
	if dotIdx >= 0 {
		prefix := expr[:dotIdx]
		partial := expr[dotIdx+1:]
		return s.completeTemplateDot(prefix, partial)
	}

	// Complete top-level names.
	var items []CompletionItem
	for _, c := range topLevel {
		if expr != "" && !strings.HasPrefix(c.name, expr) {
			continue
		}
		items = append(items, CompletionItem{
			Label:         c.name,
			Kind:          c.kind,
			Documentation: c.desc,
		})
	}
	return items
}

type templateCandidate struct {
	name string
	desc string
	kind int
}

func (s *Server) completeTemplateDot(prefix, partial string) []CompletionItem {
	// Known sub-completions.
	var candidates []templateCandidate

	switch prefix {
	case "assert":
		candidates = []templateCandidate{
			{"and", "Combine assertions with AND (assert.and <- [a, b])", CompletionItemKindFunction},
			{"or", "Combine assertions with OR (assert.or <- [a, b])", CompletionItemKindFunction},
			{"any", "Accept any value (always passes)", CompletionItemKindFunction},
			{"contains", "Assert value contains substring/element (assert.contains <- expected)", CompletionItemKindFunction},
			{"notContains", "Assert value does not contain (assert.notContains <- value)", CompletionItemKindFunction},
			{"regexp", "Assert value matches regexp pattern (assert.regexp <- pattern)", CompletionItemKindFunction},
			{"notZero", "Assert value is not zero value", CompletionItemKindFunction},
			{"greaterThan", "Assert value > threshold (assert.greaterThan <- n)", CompletionItemKindFunction},
			{"greaterThanOrEqual", "Assert value >= threshold (assert.greaterThanOrEqual <- n)", CompletionItemKindFunction},
			{"lessThan", "Assert value < threshold (assert.lessThan <- n)", CompletionItemKindFunction},
			{"lessThanOrEqual", "Assert value <= threshold (assert.lessThanOrEqual <- n)", CompletionItemKindFunction},
			{"length", "Assert length of collection (assert.length <- n)", CompletionItemKindFunction},
		}
	}

	if candidates == nil {
		return nil
	}

	var items []CompletionItem
	for _, c := range candidates {
		if partial != "" && !strings.HasPrefix(c.name, partial) {
			continue
		}
		items = append(items, CompletionItem{
			Label:         c.name,
			Kind:          c.kind,
			Documentation: c.desc,
		})
	}
	return items
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
	} else {
		sch := schema.DetectSchemaType(doc.Text)
		diagnostics = append(diagnostics, s.validateDocument(doc, sch)...)
	}

	s.sendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	})
}

func (s *Server) validateDocument(doc *document, sch *schema.Schema) []Diagnostic {
	if doc.Parsed == nil || doc.Parsed.File == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range doc.Parsed.File.Docs {
		if d.Body == nil {
			continue
		}
		s.validateNode(d.Body, sch.Fields, nil, &diags)
	}
	return diags
}

func (s *Server) validateNode(node ast.Node, fields []*schema.FieldInfo, siblingValues map[string]string, diags *[]Diagnostic) {
	if node == nil || fields == nil {
		return
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		// First pass: collect sibling values for dynamic resolution.
		siblings := make(map[string]string)
		for _, mv := range n.Values {
			if mv.Key != nil && mv.Value != nil {
				if sv, ok := mv.Value.(*ast.StringNode); ok {
					siblings[mv.Key.String()] = sv.Value
				}
			}
		}
		// Second pass: validate each key.
		for _, mv := range n.Values {
			s.validateMappingValue(mv, fields, siblings, diags)
		}
	case *ast.MappingValueNode:
		s.validateMappingValue(n, fields, siblingValues, diags)
	}
}

func (s *Server) validateMappingValue(mv *ast.MappingValueNode, fields []*schema.FieldInfo, siblings map[string]string, diags *[]Diagnostic) {
	if mv.Key == nil {
		return
	}
	keyName := mv.Key.String()
	tok := mv.Key.GetToken()

	// Find matching field in schema.
	var field *schema.FieldInfo
	for _, f := range fields {
		if f.Name == keyName {
			field = f
			break
		}
	}

	if field == nil {
		// Unknown key.
		if tok != nil {
			*diags = append(*diags, Diagnostic{
				Range: Range{
					Start: Position{Line: tok.Position.Line - 1, Character: tok.Position.Column - 1},
					End:   Position{Line: tok.Position.Line - 1, Character: tok.Position.Column - 1 + len(keyName)},
				},
				Severity: DiagnosticSeverityWarning,
				Message:  fmt.Sprintf("unknown field %q", keyName),
			})
		}
		return
	}

	// Recurse into child nodes.
	if mv.Value != nil {
		var childFields []*schema.FieldInfo
		if field.DynamicChildren != nil {
			discriminator := ""
			if field.DynamicKey != "" && siblings != nil {
				discriminator = siblings[field.DynamicKey]
			}
			childFields = field.DynamicChildren(discriminator)
		} else {
			childFields = field.Children
		}

		switch v := mv.Value.(type) {
		case *ast.MappingNode:
			s.validateNode(v, childFields, nil, diags)
		case *ast.SequenceNode:
			// For sequences with object items (e.g., steps), validate each item.
			if field.Children != nil {
				for _, item := range v.Values {
					if m, ok := item.(*ast.MappingNode); ok {
						s.validateNode(m, field.Children, nil, diags)
					}
				}
			}
		}
	}
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
