package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/goccy/go-yaml/ast"

	"github.com/scenarigo/scenarigo/internal/lsp/schema"
	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
)

// Server is the LSP server.
type Server struct {
	reader           io.Reader
	writer           io.Writer
	logger           *log.Logger
	docs             *documentStore
	config           serverConfig
	rootURI          string
	pluginSymbolsMu  sync.Mutex
	pluginSymbolsCache map[string]*pluginSymbols // source dir → exported symbols
}

// serverConfig holds user-configurable settings.
type serverConfig struct {
	Formatting bool `json:"formatting"`
}

// NewServer creates a new LSP server that communicates over stdio.
func NewServer() *Server {
	return &Server{
		reader: os.Stdin,
		writer: os.Stdout,
		logger: log.New(os.Stderr, "[scenarigo-lsp] ", log.LstdFlags),
		docs:   newDocumentStore(),
		config: serverConfig{Formatting: false},
	}
}

// Run starts the LSP server main loop.
// It blocks until the context is canceled, the input stream is closed, or
// an "exit" notification is received.
func (s *Server) Run(ctx context.Context) error {
	type readResult struct {
		body []byte
		err  error
	}
	ch := make(chan readResult, 1)

	go func() {
		reader := bufio.NewReader(s.reader)
		for {
			body, err := readMessage(reader)
			ch <- readResult{body, err}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-ch:
			if result.err != nil {
				if result.err == io.EOF {
					return nil
				}
				return result.err
			}

			var req Request
			if err := json.Unmarshal(result.body, &req); err != nil {
				s.logger.Printf("failed to unmarshal request: %v", err)
				continue
			}

			s.handleMessage(&req)
		}
	}
}

// readMessage reads a single LSP message (headers + body) from the reader.
func readMessage(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}
	return body, nil
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
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(req)
	case "textDocument/codeAction":
		s.handleCodeAction(req)
	case "textDocument/formatting":
		s.handleFormatting(req)
	case "textDocument/signatureHelp":
		s.handleSignatureHelp(req)
	case "textDocument/references":
		s.handleReferences(req)
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
	// Read initializationOptions and rootUri if present.
	var initParams struct {
		RootURI               string        `json:"rootUri"`
		InitializationOptions *serverConfig `json:"initializationOptions"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &initParams); err == nil {
			if initParams.InitializationOptions != nil {
				s.config = *initParams.InitializationOptions
			}
			s.rootURI = initParams.RootURI
		}
	}

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: 1, // Full sync.
			CompletionProvider: &CompletionOptions{
				TriggerCharacters: []string{":", " ", "\n"},
			},
			HoverProvider:              true,
			DefinitionProvider:         true,
			DocumentSymbolProvider:     true,
			CodeActionProvider:         true,
			ReferencesProvider:         true,
			DocumentFormattingProvider: s.config.Formatting,
			SignatureHelpProvider: &SignatureHelpOptions{
				TriggerCharacters: []string{"<"},
			},
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
	s.publishDiagnostics(params.TextDocument.URI)
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
		s.sendResponse(req.ID, CompletionList{Items: []CompletionItem{}}, nil)
		return
	}

	items := s.complete(doc, params.Position)
	if items == nil {
		items = []CompletionItem{}
	}
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

func (s *Server) handleDocumentSymbol(req *Request) {
	var params DocumentSymbolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("documentSymbol unmarshal error: %v", err)
		s.sendResponse(req.ID, nil, nil)
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil || doc.Parsed == nil || doc.Parsed.File == nil {
		s.sendResponse(req.ID, []DocumentSymbol{}, nil)
		return
	}

	sch := schema.DetectSchemaType(doc.Text)
	if sch == nil {
		s.sendResponse(req.ID, []DocumentSymbol{}, nil)
		return
	}

	symbols := s.documentSymbols(doc)
	s.sendResponse(req.ID, symbols, nil)
}

func (s *Server) handleCodeAction(req *Request) {
	var params CodeActionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("codeAction unmarshal error: %v", err)
		s.sendResponse(req.ID, nil, nil)
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil || doc.Parsed == nil {
		s.sendResponse(req.ID, []CodeAction{}, nil)
		return
	}

	sch := schema.DetectSchemaType(doc.Text)
	if sch == nil {
		s.sendResponse(req.ID, []CodeAction{}, nil)
		return
	}

	var actions []CodeAction

	for _, diag := range params.Context.Diagnostics {
		if !strings.HasPrefix(diag.Message, "unknown field ") {
			continue
		}

		// Extract the unknown field name from the diagnostic message.
		unknownKey := strings.TrimPrefix(diag.Message, `unknown field "`)
		unknownKey = strings.TrimSuffix(unknownKey, `"`)

		// Get valid fields at this position using cursor context.
		ctx := doc.Parsed.GetCursorContext(diag.Range.Start.Line, diag.Range.Start.Character)
		if ctx == nil {
			continue
		}

		var validFields []*schema.FieldInfo
		if ctx.Type == yamlutil.CursorContextKey || ctx.Type == yamlutil.CursorContextUnknown {
			validFields = sch.ChildFields(ctx.Path, ctx.SiblingValues)
		}
		if validFields == nil {
			validFields = sch.Fields
		}

		// Find similar field names.
		for _, f := range validFields {
			if levenshtein(unknownKey, f.Name) <= 3 {
				action := CodeAction{
					Title:       fmt.Sprintf("Did you mean %q?", f.Name),
					Kind:        CodeActionKindQuickFix,
					Diagnostics: []Diagnostic{diag},
					Edit: &WorkspaceEdit{
						Changes: map[string][]TextEdit{
							params.TextDocument.URI: {
								{
									Range:   diag.Range,
									NewText: f.Name,
								},
							},
						},
					},
				}
				actions = append(actions, action)
			}
		}
	}

	s.sendResponse(req.ID, actions, nil)
}

func (s *Server) handleFormatting(req *Request) {
	var params DocumentFormattingParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("formatting unmarshal error: %v", err)
		s.sendResponse(req.ID, nil, nil)
		return
	}

	if !s.config.Formatting {
		s.sendResponse(req.ID, nil, nil)
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil || doc.Parsed == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}

	sch := schema.DetectSchemaType(doc.Text)
	if sch == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}

	edits := s.formatDocument(doc, sch)
	s.sendResponse(req.ID, edits, nil)
}

func (s *Server) handleSignatureHelp(req *Request) {
	var params SignatureHelpParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("signatureHelp unmarshal error: %v", err)
		s.sendResponse(req.ID, nil, nil)
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}

	help := s.signatureHelp(doc, params.Position)
	if help == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}
	s.sendResponse(req.ID, help, nil)
}

// templateSignature describes a scenarigo template function.
type templateSignature struct {
	name   string
	label  string
	doc    string
	params []ParameterInformation
}

var templateSignatures = []templateSignature{
	{
		name: "assert.contains", label: "assert.contains <- expected",
		doc:    "Assert that the value contains the expected substring or element",
		params: []ParameterInformation{{Label: "expected", Documentation: "Substring or element to find"}},
	},
	{
		name: "assert.notContains", label: "assert.notContains <- value",
		doc:    "Assert that the value does not contain the given substring or element",
		params: []ParameterInformation{{Label: "value", Documentation: "Substring or element that should not be present"}},
	},
	{
		name: "assert.regexp", label: "assert.regexp <- pattern",
		doc:    "Assert that the value matches the regular expression pattern",
		params: []ParameterInformation{{Label: "pattern", Documentation: "Regular expression pattern"}},
	},
	{
		name: "assert.greaterThan", label: "assert.greaterThan <- threshold",
		doc:    "Assert that the value is greater than the threshold",
		params: []ParameterInformation{{Label: "threshold", Documentation: "Threshold value"}},
	},
	{
		name: "assert.greaterThanOrEqual", label: "assert.greaterThanOrEqual <- threshold",
		doc:    "Assert that the value is greater than or equal to the threshold",
		params: []ParameterInformation{{Label: "threshold", Documentation: "Threshold value"}},
	},
	{
		name: "assert.lessThan", label: "assert.lessThan <- threshold",
		doc:    "Assert that the value is less than the threshold",
		params: []ParameterInformation{{Label: "threshold", Documentation: "Threshold value"}},
	},
	{
		name: "assert.lessThanOrEqual", label: "assert.lessThanOrEqual <- threshold",
		doc:    "Assert that the value is less than or equal to the threshold",
		params: []ParameterInformation{{Label: "threshold", Documentation: "Threshold value"}},
	},
	{
		name: "assert.length", label: "assert.length <- n",
		doc:    "Assert that the collection has exactly n elements",
		params: []ParameterInformation{{Label: "n", Documentation: "Expected length"}},
	},
	{
		name: "assert.and", label: "assert.and <- [assertions...]",
		doc:    "Combine multiple assertions with AND (all must pass)",
		params: []ParameterInformation{{Label: "assertions", Documentation: "List of assertions"}},
	},
	{
		name: "assert.or", label: "assert.or <- [assertions...]",
		doc:    "Combine multiple assertions with OR (at least one must pass)",
		params: []ParameterInformation{{Label: "assertions", Documentation: "List of assertions"}},
	},
}

func (s *Server) signatureHelp(doc *document, pos Position) *SignatureHelp {
	// Only active inside {{ }} template expressions, after "<-".
	tmplExpr, ok := getTemplateContext(doc.Text, pos)
	if !ok {
		return nil
	}

	// Look for "<-" in the expression.
	arrowIdx := strings.LastIndex(tmplExpr, "<-")
	if arrowIdx < 0 {
		return nil
	}

	funcName := strings.TrimSpace(tmplExpr[:arrowIdx])

	for _, sig := range templateSignatures {
		if sig.name == funcName {
			return &SignatureHelp{
				Signatures: []SignatureInformation{
					{
						Label:         sig.label,
						Documentation: sig.doc,
						Parameters:    sig.params,
					},
				},
				ActiveSignature: 0,
				ActiveParameter: 0,
			}
		}
	}
	return nil
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func (s *Server) documentSymbols(doc *document) []DocumentSymbol {
	if doc.Parsed == nil || doc.Parsed.File == nil {
		return nil
	}
	var symbols []DocumentSymbol
	for _, d := range doc.Parsed.File.Docs {
		if d.Body == nil {
			continue
		}
		symbols = append(symbols, s.nodeToSymbols(d.Body)...)
	}
	return symbols
}

func (s *Server) nodeToSymbols(node ast.Node) []DocumentSymbol {
	if node == nil {
		return nil
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		var syms []DocumentSymbol
		for _, mv := range n.Values {
			syms = append(syms, s.mappingValueToSymbol(mv))
		}
		return syms
	case *ast.MappingValueNode:
		sym := s.mappingValueToSymbol(n)
		return []DocumentSymbol{sym}
	default:
		return nil
	}
}

func (s *Server) mappingValueToSymbol(mv *ast.MappingValueNode) DocumentSymbol {
	keyName := mv.Key.String()
	tok := mv.Key.GetToken()

	startLine := 0
	startChar := 0
	if tok != nil {
		startLine = tok.Position.Line - 1
		startChar = tok.Position.Column - 1
	}

	selRange := Range{
		Start: Position{Line: startLine, Character: startChar},
		End:   Position{Line: startLine, Character: startChar + len(keyName)},
	}

	sym := DocumentSymbol{
		Name:           keyName,
		Kind:           symbolKindForNode(mv.Value),
		Range:          nodeRange(mv),
		SelectionRange: selRange,
	}

	// Add detail for simple values.
	if mv.Value != nil {
		switch v := mv.Value.(type) {
		case *ast.StringNode:
			sym.Detail = v.Value
		case *ast.IntegerNode:
			sym.Detail = v.Token.Value
		case *ast.BoolNode:
			sym.Detail = v.Token.Value
		}
	}

	// Recurse into children.
	if mv.Value != nil {
		switch v := mv.Value.(type) {
		case *ast.MappingNode:
			for _, child := range v.Values {
				sym.Children = append(sym.Children, s.mappingValueToSymbol(child))
			}
		case *ast.SequenceNode:
			for i, item := range v.Values {
				switch m := item.(type) {
				case *ast.MappingNode:
					// Sequence item with mapping: create a symbol for the item.
					itemSym := DocumentSymbol{
						Name:           fmt.Sprintf("[%d]", i),
						Kind:           SymbolKindObject,
						Range:          nodeRange(m),
						SelectionRange: nodeRange(m),
					}
					// Use "title" or first key as the name if available.
					for _, child := range m.Values {
						if child.Key.String() == "title" {
							if sv, ok := child.Value.(*ast.StringNode); ok {
								itemSym.Name = sv.Value
							}
						}
						itemSym.Children = append(itemSym.Children, s.mappingValueToSymbol(child))
					}
					sym.Children = append(sym.Children, itemSym)
				}
			}
		}
	}

	return sym
}

func symbolKindForNode(node ast.Node) int {
	if node == nil {
		return SymbolKindProperty
	}
	switch node.(type) {
	case *ast.MappingNode:
		return SymbolKindObject
	case *ast.SequenceNode:
		return SymbolKindArray
	case *ast.StringNode:
		return SymbolKindString
	case *ast.IntegerNode:
		return SymbolKindNumber
	case *ast.BoolNode:
		return SymbolKindBoolean
	default:
		return SymbolKindProperty
	}
}

func nodeRange(node ast.Node) Range {
	tok := node.GetToken()
	if tok == nil {
		return Range{}
	}
	startLine := tok.Position.Line - 1
	startChar := tok.Position.Column - 1
	// Approximate end position from the token.
	return Range{
		Start: Position{Line: startLine, Character: startChar},
		End:   Position{Line: startLine, Character: startChar + len(tok.Value)},
	}
}

func (s *Server) definition(doc *document, params DefinitionParams) *Location {
	if doc.Parsed == nil {
		return nil
	}
	if schema.DetectSchemaType(doc.Text) == nil {
		return nil
	}

	// Check if cursor is inside a template expression (e.g., {{vars.token}}).
	if loc := s.templateVarDefinition(doc, params); loc != nil {
		return loc
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

// templateVarDefinition handles definition jump for template variable references
// like {{vars.token}} or {{secrets.apiKey}}.
// It first looks for the definition in the current document, then falls back to
// the scenarigo.yaml config file.
func (s *Server) templateVarDefinition(doc *document, params DefinitionParams) *Location {
	tmplExpr, ok := getFullTemplateExpr(doc.Text, params.Position)
	if !ok {
		return nil
	}
	tmplExpr = strings.TrimSpace(tmplExpr)
	if tmplExpr == "" {
		return nil
	}

	// Handle "<-" operator: only consider the left-hand side.
	if arrowIdx := strings.Index(tmplExpr, "<-"); arrowIdx >= 0 {
		tmplExpr = strings.TrimSpace(tmplExpr[:arrowIdx])
	}

	// For function call syntax like plugins.myplugin.CreateClient(vars.apiEndpoint),
	// resolve the sub-expression under the cursor:
	//   cursor on CreateClient → plugins.myplugin.CreateClient
	//   cursor on apiEndpoint  → vars.apiEndpoint
	resolveExpr := tmplExpr
	if parenIdx := strings.Index(tmplExpr, "("); parenIdx >= 0 {
		lines := strings.Split(doc.Text, "\n")
		if params.Position.Line < len(lines) {
			line := lines[params.Position.Line]
			prefix := line[:params.Position.Character]
			openIdx := strings.LastIndex(prefix, "{{")
			if openIdx >= 0 {
				cursorInRaw := params.Position.Character - (openIdx + 2)
				rawParenIdx := strings.Index(line[openIdx+2:], "(")
				if rawParenIdx >= 0 && cursorInRaw > rawParenIdx {
					// Cursor is inside the argument list.
					closeIdx := strings.LastIndex(tmplExpr, ")")
					if closeIdx > parenIdx {
						resolveExpr = strings.TrimSpace(tmplExpr[parenIdx+1 : closeIdx])
					}
				} else {
					// Cursor is on the function/variable part.
					resolveExpr = tmplExpr[:parenIdx]
				}
			}
		}
	}

	parts := strings.Split(resolveExpr, ".")
	if len(parts) < 2 {
		return nil
	}
	root := parts[0]
	if root != "vars" && root != "secrets" && root != "plugins" {
		return nil
	}
	name := parts[1]

	// Try to find the definition in the current document.
	if root == "plugins" {
		if len(parts) >= 3 {
			// plugins.<name>.<symbol> — jump to Go source definition.
			return s.findPluginSymbolDefinition(doc, name, parts[2])
		}
		// plugins.<name> — jump to plugin declaration.
		if r := findBlockKeyRange(doc.Text, "plugins", name); r != nil {
			return &Location{URI: params.TextDocument.URI, Range: *r}
		}
		return s.findConfigDefinition("plugins", name)
	}

	symbolPath := []string{root, name}
	if dr := findDeclRange(doc, symbolPath); dr != nil {
		return &Location{
			URI:   params.TextDocument.URI,
			Range: *dr,
		}
	}

	// Fall back to scenarigo.yaml config file.
	return s.findConfigDefinition(root, name)
}

// readConfigText returns the text of scenarigo.yaml and its URI.
// It checks the document store first, then falls back to reading from disk.
func (s *Server) readConfigText() (text, uri string, ok bool) {
	rootPath := uriToPath(s.rootURI)
	if rootPath == "" {
		return "", "", false
	}
	configPath := filepath.Join(rootPath, "scenarigo.yaml")
	configURI := pathToURI(configPath)
	if doc := s.docs.Get(configURI); doc != nil {
		return doc.Text, configURI, true
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", "", false
	}
	return string(data), configURI, true
}

// findConfigDefinition searches for a variable definition in scenarigo.yaml.
func (s *Server) findConfigDefinition(blockName, keyName string) *Location {
	configText, configURI, ok := s.readConfigText()
	if !ok {
		return nil
	}
	if r := findBlockKeyRange(configText, blockName, keyName); r != nil {
		return &Location{URI: configURI, Range: *r}
	}
	return nil
}

// findPluginSymbolDefinition finds the Go source location of an exported symbol in a plugin.
func (s *Server) findPluginSymbolDefinition(doc *document, pluginAlias, symbolName string) *Location {
	sourceDir := s.resolvePluginSourceDir(doc, pluginAlias)
	if sourceDir == "" {
		return nil
	}

	// Parse Go files to find the symbol's location.
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, sourceDir, nil, 0)
	if err != nil {
		return nil
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *goast.FuncDecl:
					if d.Recv == nil && d.Name.Name == symbolName {
						pos := fset.Position(d.Name.Pos())
						return &Location{
							URI: pathToURI(pos.Filename),
							Range: Range{
								Start: Position{Line: pos.Line - 1, Character: pos.Column - 1},
								End:   Position{Line: pos.Line - 1, Character: pos.Column - 1 + len(symbolName)},
							},
						}
					}
				case *goast.GenDecl:
					if d.Tok != token.VAR {
						continue
					}
					for _, spec := range d.Specs {
						vs, ok := spec.(*goast.ValueSpec)
						if !ok {
							continue
						}
						for _, name := range vs.Names {
							if name.Name == symbolName {
								pos := fset.Position(name.Pos())
								return &Location{
									URI: pathToURI(pos.Filename),
									Range: Range{
										Start: Position{Line: pos.Line - 1, Character: pos.Column - 1},
										End:   Position{Line: pos.Line - 1, Character: pos.Column - 1 + len(symbolName)},
									},
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// findBlockKeyRange finds the range of a specific key within a named block in YAML text.
func findBlockKeyRange(text, blockName, keyName string) *Range {
	lines := strings.Split(text, "\n")
	inBlock := false
	blockIndent := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == blockName+":" {
			inBlock = true
			blockIndent = getLineIndent(line)
			continue
		}
		if inBlock {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			lineIndent := getLineIndent(line)
			if lineIndent <= blockIndent {
				break
			}
			key := extractKeyFromLine(line)
			if key == keyName {
				r := keyRange(i, line, keyName)
				return &r
			}
		}
	}
	return nil
}

// pluginSymbol represents an exported symbol from a Go plugin source.
type pluginSymbol struct {
	Name      string
	Signature string // e.g., "func(ctx context.Context, addr string) TestClient"
	Doc       string // doc comment
	IsFunc    bool
}

// pluginSymbols holds exported symbols extracted from a Go plugin source directory.
type pluginSymbols struct {
	Symbols []pluginSymbol
}

// extractGoExportedSymbols parses Go source files in dir and returns exported symbols with signatures and docs.
func extractGoExportedSymbols(dir string) (*pluginSymbols, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var syms pluginSymbols
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *goast.FuncDecl:
					if d.Recv == nil && d.Name.IsExported() {
						syms.Symbols = append(syms.Symbols, pluginSymbol{
							Name:      d.Name.Name,
							Signature: formatFuncSignature(d),
							Doc:       cleanDocComment(d.Doc),
							IsFunc:    true,
						})
					}
				case *goast.GenDecl:
					if d.Tok != token.VAR {
						continue
					}
					for _, spec := range d.Specs {
						vs, ok := spec.(*goast.ValueSpec)
						if !ok {
							continue
						}
						for _, name := range vs.Names {
							if name.IsExported() {
								syms.Symbols = append(syms.Symbols, pluginSymbol{
									Name:      name.Name,
									Signature: formatVarType(vs),
									Doc:       cleanDocComment(firstNonNil(vs.Doc, d.Doc)),
								})
							}
						}
					}
				}
			}
		}
	}
	return &syms, nil
}

func firstNonNil(groups ...*goast.CommentGroup) *goast.CommentGroup {
	for _, g := range groups {
		if g != nil {
			return g
		}
	}
	return nil
}

// formatFuncSignature formats a function declaration's signature as a string.
func formatFuncSignature(d *goast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func(")
	if d.Type.Params != nil {
		formatFieldList(&b, d.Type.Params)
	}
	b.WriteString(")")
	if d.Type.Results != nil && len(d.Type.Results.List) > 0 {
		if len(d.Type.Results.List) == 1 && len(d.Type.Results.List[0].Names) == 0 {
			b.WriteString(" ")
			formatExpr(&b, d.Type.Results.List[0].Type)
		} else {
			b.WriteString(" (")
			formatFieldList(&b, d.Type.Results)
			b.WriteString(")")
		}
	}
	return b.String()
}

// formatVarType formats the type of a variable declaration.
func formatVarType(vs *goast.ValueSpec) string {
	if vs.Type != nil {
		var b strings.Builder
		formatExpr(&b, vs.Type)
		return b.String()
	}
	return "var"
}

// formatFieldList formats a list of function parameters or results.
func formatFieldList(b *strings.Builder, fl *goast.FieldList) {
	for i, field := range fl.List {
		if i > 0 {
			b.WriteString(", ")
		}
		if len(field.Names) > 0 {
			for j, name := range field.Names {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(name.Name)
			}
			b.WriteString(" ")
		}
		formatExpr(b, field.Type)
	}
}

// formatExpr formats a Go type expression as a string.
func formatExpr(b *strings.Builder, expr goast.Expr) {
	switch t := expr.(type) {
	case *goast.Ident:
		b.WriteString(t.Name)
	case *goast.SelectorExpr:
		formatExpr(b, t.X)
		b.WriteString(".")
		b.WriteString(t.Sel.Name)
	case *goast.StarExpr:
		b.WriteString("*")
		formatExpr(b, t.X)
	case *goast.ArrayType:
		b.WriteString("[]")
		formatExpr(b, t.Elt)
	case *goast.MapType:
		b.WriteString("map[")
		formatExpr(b, t.Key)
		b.WriteString("]")
		formatExpr(b, t.Value)
	case *goast.InterfaceType:
		b.WriteString("interface{}")
	case *goast.Ellipsis:
		b.WriteString("...")
		formatExpr(b, t.Elt)
	case *goast.FuncType:
		b.WriteString("func(")
		if t.Params != nil {
			formatFieldList(b, t.Params)
		}
		b.WriteString(")")
		if t.Results != nil && len(t.Results.List) > 0 {
			b.WriteString(" ")
			if len(t.Results.List) == 1 && len(t.Results.List[0].Names) == 0 {
				formatExpr(b, t.Results.List[0].Type)
			} else {
				b.WriteString("(")
				formatFieldList(b, t.Results)
				b.WriteString(")")
			}
		}
	default:
		b.WriteString("any")
	}
}

// cleanDocComment extracts and cleans a doc comment from an AST comment group.
func cleanDocComment(doc *goast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	return strings.TrimSpace(doc.Text())
}

// getPluginSymbols returns the exported symbols for a plugin source directory, using a cache.
func (s *Server) getPluginSymbols(sourceDir string) *pluginSymbols {
	s.pluginSymbolsMu.Lock()
	defer s.pluginSymbolsMu.Unlock()

	if s.pluginSymbolsCache != nil {
		if syms, ok := s.pluginSymbolsCache[sourceDir]; ok {
			return syms
		}
	}

	syms, err := extractGoExportedSymbols(sourceDir)
	if err != nil {
		return nil
	}

	if s.pluginSymbolsCache == nil {
		s.pluginSymbolsCache = make(map[string]*pluginSymbols)
	}
	s.pluginSymbolsCache[sourceDir] = syms
	return syms
}

// resolvePluginSourceDir resolves the Go source directory for a plugin.
// It maps a scenario plugin alias to a binary name, then looks up the source path in scenarigo.yaml.
//
// Flow: scenario "plugins: { grpc: grpc.so }" → config "plugins: { grpc.so: { src: ./plugin/src } }"
func (s *Server) resolvePluginSourceDir(doc *document, pluginAlias string) string {
	rootPath := uriToPath(s.rootURI)
	if rootPath == "" {
		return ""
	}

	// Get the binary name from the scenario's plugins block.
	binaryName := ""
	if doc != nil {
		for _, key := range extractBlockKeys(doc.Text, "plugins") {
			if key == pluginAlias {
				binaryName = extractBlockValue(doc.Text, "plugins", key)
				break
			}
		}
	}
	if binaryName == "" {
		// Maybe the alias IS the binary name (config file context).
		binaryName = pluginAlias
	}

	// Read config to find source path.
	configText, _, ok := s.readConfigText()
	if !ok {
		return ""
	}

	// Extract src value for this plugin binary from config.
	srcPath := extractPluginSrc(configText, binaryName)
	if srcPath == "" {
		return ""
	}

	// Resolve relative to config root.
	if !filepath.IsAbs(srcPath) {
		srcPath = filepath.Join(rootPath, srcPath)
	}

	// Verify directory exists.
	if info, err := os.Stat(srcPath); err != nil || !info.IsDir() {
		return ""
	}
	return srcPath
}

// extractBlockValue extracts the value for a specific key in a named block.
// e.g., extractBlockValue(text, "plugins", "grpc") returns "grpc.so" from "plugins:\n  grpc: grpc.so".
func extractBlockValue(text, blockName, keyName string) string {
	lines := strings.Split(text, "\n")
	inBlock := false
	blockIndent := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == blockName+":" {
			inBlock = true
			blockIndent = getLineIndent(line)
			continue
		}
		if inBlock {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			lineIndent := getLineIndent(line)
			if lineIndent <= blockIndent {
				break
			}
			key := extractKeyFromLine(line)
			if key == keyName {
				colonIdx := strings.Index(trimmed, ":")
				if colonIdx >= 0 {
					val := strings.TrimSpace(trimmed[colonIdx+1:])
					val = strings.Trim(val, `"'`)
					return val
				}
			}
		}
	}
	return ""
}

// extractTopLevelValue extracts the value of a top-level key from YAML text.
// e.g., extractTopLevelValue(text, "pluginDirectory") returns "./gen" from "pluginDirectory: ./gen".
func extractTopLevelValue(text, key string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+":") {
			val := strings.TrimSpace(trimmed[len(key)+1:])
			return strings.Trim(val, `"'`)
		}
	}
	return ""
}

// extractPluginSrc extracts the src value for a plugin binary from config text.
// Config format: "plugins:\n  binary.so:\n    src: ./path"
func extractPluginSrc(configText, binaryName string) string {
	lines := strings.Split(configText, "\n")
	inPlugins := false
	pluginsIndent := -1
	inPlugin := false
	pluginIndent := -1

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lineIndent := getLineIndent(line)

		if trimmed == "plugins:" {
			inPlugins = true
			pluginsIndent = lineIndent
			inPlugin = false
			continue
		}

		if inPlugins && lineIndent <= pluginsIndent {
			break
		}

		if inPlugins && !inPlugin {
			key := extractKeyFromLine(line)
			if key == binaryName {
				inPlugin = true
				pluginIndent = lineIndent
				// Check if src is on the same line (inline mapping).
				continue
			}
		}

		if inPlugin {
			if lineIndent <= pluginIndent {
				break // Left this plugin's block.
			}
			key := extractKeyFromLine(line)
			if key == "src" {
				colonIdx := strings.Index(trimmed, ":")
				if colonIdx >= 0 {
					val := strings.TrimSpace(trimmed[colonIdx+1:])
					val = strings.Trim(val, `"'`)
					return val
				}
			}
		}
	}
	return ""
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
	sch := schema.DetectSchemaType(doc.Text)
	if sch == nil {
		return nil
	}

	// Check if cursor is inside a template expression {{ }}.
	if tmplExpr, ok := getTemplateContext(doc.Text, pos); ok {
		return s.completeTemplate(doc, tmplExpr)
	}

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
		items := s.completeKeys(sch, ctx, pos)
		if items != nil {
			return items
		}
		// If no key completions found, the parent might be a file-path array
		// (e.g. "scenarios: \n  - <cursor>"). Fall through to file path completion.
		return s.completeFilePathFromContext(sch, ctx, doc.URI)
	case yamlutil.CursorContextValue:
		return s.completeValues(sch, ctx, doc.URI)
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

func (s *Server) completeTemplate(doc *document, expr string) []CompletionItem {
	// Parse the expression to determine what to complete.
	// "vars" → complete top-level template variables
	// "vars." → complete after dot
	// "" → complete all top-level names

	// For function call syntax like "plugins.myplugin.CreateClient(vars.api",
	// complete the argument sub-expression after the last "(".
	if parenIdx := strings.LastIndex(expr, "("); parenIdx >= 0 {
		expr = strings.TrimSpace(expr[parenIdx+1:])
	}

	// Top-level template names available in scenarigo.
	topLevel := []templateCandidate{
		{name: "vars", detail: "Scenario/step variables", kind: CompletionItemKindVariable},
		{name: "secrets", detail: "Secret variables", kind: CompletionItemKindVariable},
		{name: "plugins", detail: "Plugin exports", kind: CompletionItemKindVariable},
		{name: "request", detail: "Request data (protocol-specific)", kind: CompletionItemKindVariable},
		{name: "response", detail: "Response data (protocol-specific)", kind: CompletionItemKindVariable},
		{name: "steps", detail: "Results from previous steps", kind: CompletionItemKindVariable},
		{name: "env", detail: "Environment variable", kind: CompletionItemKindVariable},
		{name: "assert", detail: "Assertion functions", kind: CompletionItemKindModule},
		{name: "size", detail: "func(v any) int", doc: "Get size of collection", kind: CompletionItemKindFunction},
		{name: "type", detail: "func(v any) string", doc: "Get type name of value", kind: CompletionItemKindFunction},
		{name: "int", detail: "func(v any) int64", doc: "Convert value to int", kind: CompletionItemKindFunction},
		{name: "uint", detail: "func(v any) uint64", doc: "Convert value to uint", kind: CompletionItemKindFunction},
		{name: "float", detail: "func(v any) float64", doc: "Convert value to float", kind: CompletionItemKindFunction},
		{name: "bool", detail: "func(v any) bool", doc: "Convert value to bool", kind: CompletionItemKindFunction},
		{name: "string", detail: "func(v any) string", doc: "Convert value to string", kind: CompletionItemKindFunction},
		{name: "bytes", detail: "func(v any) []byte", doc: "Convert value to bytes", kind: CompletionItemKindFunction},
		{name: "time", detail: "func(v any) time.Time", doc: "Convert value to time.Time", kind: CompletionItemKindFunction},
		{name: "duration", detail: "func(v any) time.Duration", doc: "Convert value to time.Duration", kind: CompletionItemKindFunction},
	}

	// If there's a dot, we need to complete after the prefix.
	dotIdx := strings.LastIndex(expr, ".")
	if dotIdx >= 0 {
		prefix := expr[:dotIdx]
		partial := expr[dotIdx+1:]
		return s.completeTemplateDot(doc, prefix, partial)
	}

	// Complete top-level names.
	var items []CompletionItem
	for _, c := range topLevel {
		if expr != "" && !strings.HasPrefix(c.name, expr) {
			continue
		}
		doc := c.doc
		if doc == "" {
			doc = c.detail
		}
		items = append(items, CompletionItem{
			Label:         c.name,
			Kind:          c.kind,
			Detail:        c.detail,
			Documentation: doc,
		})
	}
	return items
}

type templateCandidate struct {
	name   string
	detail string // short signature shown inline (e.g., "func(expected any)")
	doc    string // longer documentation (e.g., doc comment)
	kind   int
}

func (s *Server) completeTemplateDot(doc *document, prefix, partial string) []CompletionItem {
	// Known sub-completions.
	var candidates []templateCandidate

	switch prefix {
	case "assert":
		candidates = []templateCandidate{
			{name: "and", detail: "func(assertions ...any) any", doc: "Combine multiple assertions with AND — all must pass", kind: CompletionItemKindFunction},
			{name: "or", detail: "func(assertions ...any) any", doc: "Combine multiple assertions with OR — at least one must pass", kind: CompletionItemKindFunction},
			{name: "any", detail: "any", doc: "Accept any value (always passes)", kind: CompletionItemKindFunction},
			{name: "contains", detail: "func(expected any) any", doc: "Assert value contains the expected substring or element", kind: CompletionItemKindFunction},
			{name: "notContains", detail: "func(value any) any", doc: "Assert value does not contain the given substring or element", kind: CompletionItemKindFunction},
			{name: "regexp", detail: "func(pattern string) any", doc: "Assert value matches the regular expression pattern", kind: CompletionItemKindFunction},
			{name: "notZero", detail: "any", doc: "Assert value is not the zero value of its type", kind: CompletionItemKindFunction},
			{name: "greaterThan", detail: "func(n any) any", doc: "Assert value is greater than the threshold", kind: CompletionItemKindFunction},
			{name: "greaterThanOrEqual", detail: "func(n any) any", doc: "Assert value is greater than or equal to the threshold", kind: CompletionItemKindFunction},
			{name: "lessThan", detail: "func(n any) any", doc: "Assert value is less than the threshold", kind: CompletionItemKindFunction},
			{name: "lessThanOrEqual", detail: "func(n any) any", doc: "Assert value is less than or equal to the threshold", kind: CompletionItemKindFunction},
			{name: "length", detail: "func(n int) any", doc: "Assert collection has the specified length", kind: CompletionItemKindFunction},
		}
	case "vars", "secrets":
		if doc != nil {
			for _, key := range extractBlockKeys(doc.Text, prefix) {
				candidates = append(candidates, templateCandidate{name: key, kind: CompletionItemKindVariable})
			}
			// Also include keys from bind.vars / bind.secrets blocks.
			for _, bk := range extractBindKeys(doc.Text, prefix) {
				candidates = append(candidates, templateCandidate{name: bk.key, detail: "(from bind)", kind: CompletionItemKindVariable})
			}
		}
		// Also look for keys in the config file (scenarigo.yaml).
		for _, key := range s.configBlockKeys(prefix) {
			candidates = append(candidates, templateCandidate{name: key, detail: "(from scenarigo.yaml)", kind: CompletionItemKindVariable})
		}
	case "steps":
		if doc != nil {
			for _, id := range extractStepIDs(doc.Text) {
				candidates = append(candidates, templateCandidate{name: id, detail: "Step result", kind: CompletionItemKindVariable})
			}
		}
	case "plugins":
		if doc != nil {
			for _, name := range extractBlockKeys(doc.Text, "plugins") {
				candidates = append(candidates, templateCandidate{name: name, detail: "Plugin", kind: CompletionItemKindModule})
			}
		}
		// Also look for plugins defined in the config file.
		for _, name := range s.configBlockKeys("plugins") {
			candidates = append(candidates, templateCandidate{name: name, detail: "Plugin (from scenarigo.yaml)", kind: CompletionItemKindModule})
		}
	default:
		// Handle plugins.<name> — complete exported symbols from plugin source.
		if strings.HasPrefix(prefix, "plugins.") {
			pluginAlias := strings.TrimPrefix(prefix, "plugins.")
			if sourceDir := s.resolvePluginSourceDir(doc, pluginAlias); sourceDir != "" {
				if syms := s.getPluginSymbols(sourceDir); syms != nil {
					for _, sym := range syms.Symbols {
						kind := CompletionItemKindVariable
						if sym.IsFunc {
							kind = CompletionItemKindFunction
						}
						candidates = append(candidates, templateCandidate{
							name:   sym.Name,
							detail: sym.Signature,
							doc:    sym.Doc,
							kind:   kind,
						})
					}
				}
			}
		}
	}

	if candidates == nil {
		return nil
	}

	seen := make(map[string]bool)
	var items []CompletionItem
	for _, c := range candidates {
		if partial != "" && !strings.HasPrefix(c.name, partial) {
			continue
		}
		if seen[c.name] {
			continue
		}
		seen[c.name] = true
		doc := c.doc
		if doc == "" {
			doc = c.detail
		}
		items = append(items, CompletionItem{
			Label:         c.name,
			Kind:          c.kind,
			Detail:        c.detail,
			Documentation: doc,
		})
	}
	return items
}

func (s *Server) completeKeys(sch *schema.Schema, ctx *yamlutil.CursorContext, pos Position) []CompletionItem {
	fields := sch.ChildFields(ctx.Path, ctx.SiblingValues)
	if fields == nil {
		return nil
	}

	// Filter out keys that already exist at this level.
	existing := make(map[string]bool)
	for _, k := range ctx.ParentKeys {
		existing[k] = true
	}

	// TextEdit range: from the start of the partial key to the cursor position.
	startChar := pos.Character - len(ctx.PartialKey)
	editRange := Range{
		Start: Position{Line: pos.Line, Character: startChar},
		End:   Position{Line: pos.Line, Character: pos.Character},
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
			TextEdit:      &TextEdit{Range: editRange, NewText: insertText},
			SortText:      fmt.Sprintf("%03d_%s", i, f.Name),
		})
	}
	return items
}

func (s *Server) completeValues(sch *schema.Schema, ctx *yamlutil.CursorContext, docURI string) []CompletionItem {
	if len(ctx.Path) == 0 {
		return nil
	}

	field := sch.FindField(ctx.Path)
	if field == nil {
		// For map-type fields like plugins, the full path (e.g., ["plugins", "myplugin"])
		// won't match a schema field. Check if the parent is a file-path map field.
		if len(ctx.Path) >= 2 {
			parentField := sch.FindField(ctx.Path[:len(ctx.Path)-1])
			if parentField != nil && parentField.Type == schema.FieldTypeMap && parentField.IsFilePath {
				if dir := s.resolvePluginBuildDir(); dir != "" {
					return completeFilePathInDir(dir, ctx.PartialValue)
				}
				return s.completeFilePath(docURI, ctx.PartialValue)
			}
		}
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

	// File path completion.
	if field.IsFilePath {
		return s.completeFilePath(docURI, ctx.PartialValue)
	}

	return nil
}

// completeFilePathFromContext checks if the current path refers to a file-path
// array field (e.g., scenarios) and offers filesystem completion.
func (s *Server) completeFilePathFromContext(sch *schema.Schema, ctx *yamlutil.CursorContext, docURI string) []CompletionItem {
	if len(ctx.Path) == 0 {
		return nil
	}
	field := sch.FindField(ctx.Path)
	if field == nil || !field.IsFilePath {
		return nil
	}
	return s.completeFilePath(docURI, ctx.PartialKey)
}

func (s *Server) completeFilePath(docURI, partial string) []CompletionItem {
	docPath := uriToPath(docURI)
	if docPath == "" {
		return nil
	}
	return completeFilePathInDir(filepath.Dir(docPath), partial)
}

// resolvePluginBuildDir returns the absolute path of pluginDirectory from config.
func (s *Server) resolvePluginBuildDir() string {
	configText, _, ok := s.readConfigText()
	if !ok {
		return ""
	}
	rootPath := uriToPath(s.rootURI)

	dir := extractTopLevelValue(configText, "pluginDirectory")
	if dir == "" {
		return ""
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(rootPath, dir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// completeFilePathInDir lists files in the given base directory for file path completion.
func completeFilePathInDir(dir, partial string) []CompletionItem {
	searchDir := dir
	prefix := partial
	if partial != "" {
		absPartial := filepath.Join(dir, partial)
		info, err := os.Stat(absPartial)
		if err == nil && info.IsDir() {
			searchDir = absPartial
			prefix = ""
		} else {
			searchDir = filepath.Dir(absPartial)
			prefix = filepath.Base(absPartial)
		}
	}

	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}

	relDir, err := filepath.Rel(dir, searchDir)
	if err != nil {
		return nil
	}

	var items []CompletionItem
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		relPath := name
		if relDir != "." {
			relPath = filepath.Join(relDir, name)
		}
		kind := CompletionItemKindFile
		if entry.IsDir() {
			kind = CompletionItemKindFolder
			relPath += "/"
		}
		items = append(items, CompletionItem{
			Label:      relPath,
			Kind:       kind,
			InsertText: relPath,
		})
	}
	return items
}

func (s *Server) formatDocument(doc *document, sch *schema.Schema) []TextEdit {
	if doc.Parsed == nil || doc.Parsed.File == nil {
		return nil
	}

	lines := strings.Split(doc.Text, "\n")
	var edits []TextEdit

	for _, d := range doc.Parsed.File.Docs {
		if d.Body == nil {
			continue
		}
		edits = append(edits, s.formatNode(d.Body, sch.Fields, lines)...)
	}
	return edits
}

// formatNode reorders keys in a mapping to match the schema field order.
func (s *Server) formatNode(node ast.Node, fields []*schema.FieldInfo, lines []string) []TextEdit {
	if node == nil || fields == nil {
		return nil
	}

	mapping, ok := node.(*ast.MappingNode)
	if !ok {
		return nil
	}

	if len(mapping.Values) <= 1 {
		return nil // Nothing to reorder.
	}

	// Build schema order map.
	order := make(map[string]int, len(fields))
	for i, f := range fields {
		order[f.Name] = i
	}

	// Check if already in order.
	inOrder := true
	lastOrder := -1
	for _, mv := range mapping.Values {
		if mv.Key == nil {
			continue
		}
		idx, exists := order[mv.Key.String()]
		if !exists {
			idx = len(fields) // Unknown keys go at the end.
		}
		if idx < lastOrder {
			inOrder = false
			break
		}
		lastOrder = idx
	}
	if inOrder {
		// Already in schema order. Recurse into children.
		return s.formatChildren(mapping, fields, lines)
	}

	// Skip reordering if the mapping has cross-entry anchor/alias dependencies,
	// because reordering could place an alias before its anchor definition.
	if hasAnchorAliasDependency(mapping) {
		return s.formatChildren(mapping, fields, lines)
	}

	// Determine line ranges for each mapping value.
	// Pass 1: compute startLine (including leading comments) for all entries.
	type entry struct {
		mv        *ast.MappingValueNode
		keyLine   int // 0-based, the actual key line
		startLine int // 0-based inclusive (may include leading comments)
		endLine   int // 0-based inclusive
		order     int
	}

	var entries []entry
	for i, mv := range mapping.Values {
		if mv.Key == nil {
			continue
		}
		tok := mv.Key.GetToken()
		if tok == nil {
			return nil // Can't determine position, bail out.
		}
		keyLine := tok.Position.Line - 1 // Convert to 0-based.
		indent := getIndentFromLine(lines, keyLine)

		// Include leading comment lines that are contiguous with this key
		// at the same or deeper indentation.
		startLine := findLeadingCommentStart(lines, keyLine, indent)

		idx, exists := order[mv.Key.String()]
		if !exists {
			idx = len(fields) + i // Preserve relative order of unknown keys.
		}
		entries = append(entries, entry{mv: mv, keyLine: keyLine, startLine: startLine, endLine: 0, order: idx})
		_ = i // used above
	}

	if len(entries) == 0 {
		return nil
	}

	// Pass 2: compute endLine for each entry.
	// For non-last entries: endLine = next entry's startLine - 1, trimming trailing blank lines.
	// For the last entry: use findEntryEnd heuristic.
	for i := range entries {
		if i+1 < len(entries) {
			endLine := entries[i+1].startLine - 1
			// Trim trailing blank lines so they stay as separators rather than
			// being attached to this entry and moved during reordering.
			for endLine > entries[i].keyLine && strings.TrimSpace(lines[endLine]) == "" {
				endLine--
			}
			entries[i].endLine = endLine
		} else {
			entries[i].endLine = s.findEntryEnd(lines, entries[i].keyLine, getIndentFromLine(lines, entries[i].keyLine))
		}
	}

	// Sort entries by schema order (stable to preserve unknown key order).
	sorted := make([]entry, len(entries))
	copy(sorted, entries)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].order < sorted[j-1].order; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	// Check if sort actually changed anything.
	changed := false
	for i := range entries {
		if entries[i].startLine != sorted[i].startLine {
			changed = true
			break
		}
	}
	if !changed {
		return s.formatChildren(mapping, fields, lines)
	}

	// Build the replacement text.
	rangeStart := entries[0].startLine
	rangeEnd := entries[len(entries)-1].endLine

	var newLines []string
	for _, e := range sorted {
		for l := e.startLine; l <= e.endLine && l < len(lines); l++ {
			newLines = append(newLines, lines[l])
		}
	}

	edit := TextEdit{
		Range: Range{
			Start: Position{Line: rangeStart, Character: 0},
			End:   Position{Line: rangeEnd + 1, Character: 0},
		},
		NewText: strings.Join(newLines, "\n") + "\n",
	}

	return []TextEdit{edit}
}

func (s *Server) formatChildren(mapping *ast.MappingNode, fields []*schema.FieldInfo, lines []string) []TextEdit {
	// Build siblings map for DynamicChildren resolution.
	siblings := make(map[string]string)
	for _, mv := range mapping.Values {
		if mv.Key == nil || mv.Value == nil {
			continue
		}
		if sv, ok := mv.Value.(*ast.StringNode); ok {
			siblings[mv.Key.String()] = sv.Value
		}
	}

	var edits []TextEdit
	for _, mv := range mapping.Values {
		if mv.Key == nil || mv.Value == nil {
			continue
		}
		keyName := mv.Key.String()

		// Find the field definition for this key.
		var field *schema.FieldInfo
		for _, f := range fields {
			if f.Name == keyName {
				field = f
				break
			}
		}
		if field == nil {
			continue
		}

		// Resolve child fields (static or dynamic).
		var childFields []*schema.FieldInfo
		if field.DynamicChildren != nil {
			discriminator := ""
			if field.DynamicKey != "" {
				discriminator = siblings[field.DynamicKey]
			}
			childFields = field.DynamicChildren(discriminator)
		} else {
			childFields = field.Children
		}

		if childFields != nil {
			edits = append(edits, s.formatNode(mv.Value, childFields, lines)...)
		}
		// Handle sequence items (e.g., steps).
		if seq, ok := mv.Value.(*ast.SequenceNode); ok && field.Children != nil {
			for _, item := range seq.Values {
				edits = append(edits, s.formatNode(item, field.Children, lines)...)
			}
		}
	}
	return edits
}

func (s *Server) findEntryEnd(lines []string, startLine, indent int) int {
	inBlockScalar := false
	blockScalarBaseIndent := 0
	for i := startLine + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if inBlockScalar {
			if trimmed == "" {
				continue // blank lines are part of block scalar
			}
			lineIndent := len(line) - len(strings.TrimLeft(line, " "))
			if lineIndent > blockScalarBaseIndent {
				continue // still inside block scalar content
			}
			// Block scalar ended, fall through to normal processing.
			inBlockScalar = false
		}

		if trimmed == "" {
			continue
		}
		lineIndent := len(line) - len(strings.TrimLeft(line, " "))

		// Check if this line introduces a block scalar value (e.g., "key: |" or "key: >-").
		if isBlockScalarLine(trimmed) {
			inBlockScalar = true
			blockScalarBaseIndent = lineIndent
			continue
		}

		if lineIndent <= indent && !strings.HasPrefix(trimmed, "#") {
			return i - 1
		}
	}
	// Last line of file.
	end := len(lines) - 1
	for end > startLine && strings.TrimSpace(lines[end]) == "" {
		end--
	}
	return end
}

// isBlockScalarLine checks if a trimmed line contains a block scalar indicator
// as a value (e.g., "key: |", "body: >-", "key: |2").
func isBlockScalarLine(trimmed string) bool {
	colonIdx := strings.Index(trimmed, ":")
	if colonIdx < 0 {
		return false
	}
	after := strings.TrimSpace(trimmed[colonIdx+1:])
	if after == "" {
		return false
	}
	// Strip trailing comment (e.g., "| # comment").
	if commentIdx := strings.Index(after, " #"); commentIdx >= 0 {
		after = strings.TrimSpace(after[:commentIdx])
	}
	// Valid block scalar indicators: |, >, |+, |-, >+, >-, |2, >2, |+2, |-2, etc.
	if after == "" {
		return false
	}
	if after[0] != '|' && after[0] != '>' {
		return false
	}
	for _, ch := range after[1:] {
		if ch != '+' && ch != '-' && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

// findLeadingCommentStart scans backward from keyLine to find contiguous comment
// lines at the same or deeper indent. Returns the first comment line, or keyLine
// if no leading comments are found.
func findLeadingCommentStart(lines []string, keyLine, indent int) int {
	start := keyLine
	for i := keyLine - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			break // blank line ends the comment block
		}
		if !strings.HasPrefix(trimmed, "#") {
			break // not a comment
		}
		lineIndent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		if lineIndent < indent {
			break // comment at a shallower indent belongs to a parent
		}
		start = i
	}
	return start
}

// hasAnchorAliasDependency checks if any anchor defined in one entry of the mapping
// is referenced by an alias in a different entry. Reordering such a mapping could
// place an alias before its anchor, producing invalid YAML.
func hasAnchorAliasDependency(mapping *ast.MappingNode) bool {
	type anchorInfo struct {
		entryIdx int
	}
	anchors := make(map[string]int) // anchor name -> entry index

	for i, mv := range mapping.Values {
		ast.Walk(&anchorCollector{anchors: anchors, idx: i}, mv)
	}

	if len(anchors) == 0 {
		return false
	}

	// Check if any alias references an anchor from a different entry.
	for i, mv := range mapping.Values {
		var found bool
		ast.Walk(&aliasChecker{anchors: anchors, idx: i, found: &found}, mv)
		if found {
			return true
		}
	}
	return false
}

type anchorCollector struct {
	anchors map[string]int
	idx     int
}

func (c *anchorCollector) Visit(node ast.Node) ast.Visitor {
	if n, ok := node.(*ast.AnchorNode); ok && n.Name != nil {
		c.anchors[n.Name.String()] = c.idx
	}
	return c
}

type aliasChecker struct {
	anchors map[string]int
	idx     int
	found   *bool
}

func (c *aliasChecker) Visit(node ast.Node) ast.Visitor {
	if *c.found {
		return nil
	}
	if n, ok := node.(*ast.AliasNode); ok && n.Value != nil {
		if anchorIdx, exists := c.anchors[n.Value.String()]; exists && anchorIdx != c.idx {
			*c.found = true
			return nil
		}
	}
	return c
}

func getIndentFromLine(lines []string, line int) int {
	if line >= len(lines) {
		return 0
	}
	return len(lines[line]) - len(strings.TrimLeft(lines[line], " "))
}

func (s *Server) hover(doc *document, pos Position) *Hover {
	sch := schema.DetectSchemaType(doc.Text)
	if sch == nil {
		return nil
	}
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

	diagnostics := []Diagnostic{}

	sch := schema.DetectSchemaType(doc.Text)
	if sch == nil {
		// Not a scenarigo YAML file; send empty diagnostics and stay silent.
	} else if doc.Parsed == nil {
		// Check if YAML parsing failed.
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 0},
			},
			Severity: DiagnosticSeverityError,
			Message:  "Invalid YAML syntax",
		})
	} else {
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
		// First pass: collect sibling values and present keys for dynamic resolution.
		siblings := make(map[string]string)
		presentKeys := make(map[string]bool)
		for _, mv := range n.Values {
			if mv.Key != nil {
				presentKeys[mv.Key.String()] = true
				if mv.Value != nil {
					if sv, ok := mv.Value.(*ast.StringNode); ok {
						siblings[mv.Key.String()] = sv.Value
					}
				}
			}
		}
		// Second pass: validate each key.
		for _, mv := range n.Values {
			s.validateMappingValue(mv, fields, siblings, diags)
		}
		// Third pass: check required fields.
		s.validateRequiredFields(n, fields, presentKeys, diags)
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

	// Validate enum values.
	if len(field.EnumValues) > 0 && mv.Value != nil {
		if sv, ok := mv.Value.(*ast.StringNode); ok {
			valid := false
			for _, ev := range field.EnumValues {
				if sv.Value == ev {
					valid = true
					break
				}
			}
			if !valid {
				valTok := mv.Value.GetToken()
				if valTok != nil {
					*diags = append(*diags, Diagnostic{
						Range: Range{
							Start: Position{Line: valTok.Position.Line - 1, Character: valTok.Position.Column - 1},
							End:   Position{Line: valTok.Position.Line - 1, Character: valTok.Position.Column - 1 + len(sv.Value)},
						},
						Severity: DiagnosticSeverityWarning,
						Message:  fmt.Sprintf("invalid value %q for field %q (allowed: %s)", sv.Value, keyName, strings.Join(field.EnumValues, ", ")),
					})
				}
			}
		}
	}

	// Validate type.
	if mv.Value != nil {
		s.validateFieldType(mv.Value, field, keyName, diags)
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

func (s *Server) validateRequiredFields(node *ast.MappingNode, fields []*schema.FieldInfo, presentKeys map[string]bool, diags *[]Diagnostic) {
	for _, f := range fields {
		if !f.Required || presentKeys[f.Name] {
			continue
		}
		// Report at the mapping node's position.
		tok := node.GetToken()
		if tok == nil {
			continue
		}
		*diags = append(*diags, Diagnostic{
			Range: Range{
				Start: Position{Line: tok.Position.Line - 1, Character: tok.Position.Column - 1},
				End:   Position{Line: tok.Position.Line - 1, Character: tok.Position.Column - 1},
			},
			Severity: DiagnosticSeverityWarning,
			Message:  fmt.Sprintf("missing required field %q", f.Name),
		})
	}
}

func (s *Server) validateFieldType(value ast.Node, field *schema.FieldInfo, keyName string, diags *[]Diagnostic) {
	if field.Type == schema.FieldTypeAny || field.Type == schema.FieldTypeMap {
		return // Accept anything.
	}

	// Skip type checking for template expressions and alias nodes.
	switch v := value.(type) {
	case *ast.StringNode:
		if strings.Contains(v.Value, "{{") {
			return
		}
	case *ast.LiteralNode:
		return // Block scalars are strings, compatible with string fields.
	case *ast.AliasNode:
		return // Cannot determine type statically.
	case *ast.AnchorNode:
		if v.Value != nil {
			s.validateFieldType(v.Value, field, keyName, diags)
		}
		return
	}

	var mismatch string
	switch field.Type {
	case schema.FieldTypeBool:
		switch value.(type) {
		case *ast.BoolNode:
			// OK.
		case *ast.StringNode:
			// Strings are accepted (template expressions, etc.).
		default:
			mismatch = describeNodeType(value)
		}
	case schema.FieldTypeString, schema.FieldTypeDuration:
		switch value.(type) {
		case *ast.StringNode, *ast.IntegerNode, *ast.FloatNode, *ast.BoolNode:
			// OK: YAML scalars are acceptable as strings.
		default:
			mismatch = describeNodeType(value)
		}
	case schema.FieldTypeInt:
		switch value.(type) {
		case *ast.IntegerNode:
			// OK.
		case *ast.StringNode:
			// Strings are accepted (template expressions).
		default:
			mismatch = describeNodeType(value)
		}
	case schema.FieldTypeFloat:
		switch value.(type) {
		case *ast.IntegerNode, *ast.FloatNode:
			// OK.
		case *ast.StringNode:
			// Strings are accepted (template expressions).
		default:
			mismatch = describeNodeType(value)
		}
	case schema.FieldTypeObject:
		switch value.(type) {
		case *ast.MappingNode:
			// OK.
		case *ast.StringNode:
			// Strings are accepted (template expressions like "{{vars.xxx}}").
		case *ast.NullNode:
			// OK: null is acceptable for optional objects.
		default:
			mismatch = describeNodeType(value)
		}
	case schema.FieldTypeArray:
		switch value.(type) {
		case *ast.SequenceNode:
			// OK.
		case *ast.StringNode:
			// Strings are accepted (template expressions).
		case *ast.NullNode:
			// OK.
		default:
			mismatch = describeNodeType(value)
		}
	}

	if mismatch != "" {
		tok := value.GetToken()
		if tok != nil {
			*diags = append(*diags, Diagnostic{
				Range: Range{
					Start: Position{Line: tok.Position.Line - 1, Character: tok.Position.Column - 1},
					End:   Position{Line: tok.Position.Line - 1, Character: tok.Position.Column - 1 + len(value.String())},
				},
				Severity: DiagnosticSeverityWarning,
				Message:  fmt.Sprintf("field %q expects %s, got %s", keyName, field.Type, mismatch),
			})
		}
	}
}

func describeNodeType(node ast.Node) string {
	switch node.(type) {
	case *ast.BoolNode:
		return "bool"
	case *ast.IntegerNode:
		return "int"
	case *ast.FloatNode:
		return "float"
	case *ast.StringNode, *ast.LiteralNode:
		return "string"
	case *ast.MappingNode:
		return "object"
	case *ast.SequenceNode:
		return "array"
	case *ast.NullNode:
		return "null"
	default:
		return "unknown"
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

// --- textDocument/references ---

func (s *Server) handleReferences(req *Request) {
	var params ReferenceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("references unmarshal error: %v", err)
		s.sendResponse(req.ID, nil, nil)
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(req.ID, []Location{}, nil)
		return
	}

	locs := s.references(doc, params)
	if locs == nil {
		locs = []Location{}
	}
	s.sendResponse(req.ID, locs, nil)
}

// templateRef represents a reference to a dotted path inside a template expression.
type templateRef struct {
	path     []string // e.g., ["vars", "myVar"] or ["steps", "login", "response"]
	line     int      // 0-based
	startCol int      // 0-based, column of path start (e.g., "vars" in "{{vars.myVar}}")
	endCol   int      // 0-based, column past the last char of the matched prefix
}

// scanTemplateRefs scans the document text for all template expressions and extracts
// dotted path references from them.
func scanTemplateRefs(text string) []templateRef {
	var refs []templateRef
	lines := strings.Split(text, "\n")

	for lineIdx, line := range lines {
		pos := 0
		for {
			openIdx := strings.Index(line[pos:], "{{")
			if openIdx < 0 {
				break
			}
			openIdx += pos
			closeIdx := strings.Index(line[openIdx+2:], "}}")
			if closeIdx < 0 {
				break
			}
			closeIdx += openIdx + 2

			expr := line[openIdx+2 : closeIdx]

			// Handle "<-" operator: only process the left-hand side.
			if arrowIdx := strings.Index(expr, "<-"); arrowIdx >= 0 {
				expr = expr[:arrowIdx]
			}

			expr = strings.TrimSpace(expr)
			if expr == "" {
				pos = closeIdx + 2
				continue
			}

			// Parse dotted path.
			parts := strings.Split(expr, ".")
			if len(parts) == 0 {
				pos = closeIdx + 2
				continue
			}

			// Find the column where this path starts within the line.
			// The path starts after "{{" + any leading whitespace.
			exprStart := openIdx + 2
			for exprStart < closeIdx && line[exprStart] == ' ' {
				exprStart++
			}

			// Calculate end column: covers parts[0].parts[1] (the first 2 segments).
			pathStr := expr
			endColOffset := exprStart + len(pathStr)

			refs = append(refs, templateRef{
				path:     parts,
				line:     lineIdx,
				startCol: exprStart,
				endCol:   endColOffset,
			})

			pos = closeIdx + 2
		}
	}
	return refs
}

// identifySymbol determines what symbol the cursor is on.
// Returns a 2-element path like ["vars", "myVar"] or ["steps", "login"], and the declaration range.
func identifySymbol(doc *document, pos Position) (symbolPath []string, declRange *Range) {
	if doc.Parsed == nil {
		return nil, nil
	}

	// First check if cursor is inside a template expression.
	// Use getFullTemplateExpr to get the complete expression (not just up to cursor).
	if tmplExpr, ok := getFullTemplateExpr(doc.Text, pos); ok {
		tmplExpr = strings.TrimSpace(tmplExpr)
		if tmplExpr == "" {
			return nil, nil
		}
		// Handle "<-" operator: only consider the left-hand side.
		if arrowIdx := strings.Index(tmplExpr, "<-"); arrowIdx >= 0 {
			tmplExpr = strings.TrimSpace(tmplExpr[:arrowIdx])
		}
		parts := strings.Split(tmplExpr, ".")
		if len(parts) >= 2 {
			root := parts[0]
			if root == "vars" || root == "secrets" || root == "steps" {
				sp := []string{root, parts[1]}
				dr := findDeclRange(doc, sp)
				return sp, dr
			}
		}
		return nil, nil
	}

	// Check YAML structure: is cursor on a vars/secrets key or step id value?
	ctx := doc.Parsed.GetCursorContext(pos.Line, pos.Character)
	if ctx == nil || len(ctx.Path) == 0 {
		return nil, nil
	}

	lines := strings.Split(doc.Text, "\n")
	currentLine := ""
	if pos.Line < len(lines) {
		currentLine = lines[pos.Line]
	}

	// Check if cursor is on a child key of vars/secrets.
	if len(ctx.Path) >= 1 {
		parent := ctx.Path[len(ctx.Path)-1]
		if (parent == "vars" || parent == "secrets") && ctx.Type == yamlutil.CursorContextKey {
			keyName := extractKeyFromLine(currentLine)
			if keyName != "" {
				r := keyRange(pos.Line, currentLine, keyName)
				return []string{parent, keyName}, &r
			}
		}
	}

	// Check if this is a key under vars/secrets when we're on the value side.
	if len(ctx.Path) >= 2 {
		grandParent := ctx.Path[len(ctx.Path)-2]
		if grandParent == "vars" || grandParent == "secrets" {
			keyName := ctx.Path[len(ctx.Path)-1]
			r := keyRange(pos.Line, currentLine, keyName)
			return []string{grandParent, keyName}, &r
		}
	}

	// Case 2: Cursor on a step's "id" value.
	if ctx.Type == yamlutil.CursorContextValue {
		lastKey := ctx.Path[len(ctx.Path)-1]
		if lastKey == "id" && ctx.PartialValue != "" {
			// Verify it's under steps.
			for _, key := range ctx.Path {
				if key == "steps" {
					idValue := ctx.PartialValue
					r := valueRange(pos.Line, currentLine, idValue)
					return []string{"steps", idValue}, &r
				}
			}
		}
	}

	// Case 3: Cursor on the key "id" itself, get the value.
	if ctx.Type == yamlutil.CursorContextKey {
		keyName := extractKeyFromLine(currentLine)
		if keyName == "id" {
			// Get the value from the same line.
			colonIdx := strings.Index(currentLine, ":")
			if colonIdx >= 0 {
				val := strings.TrimSpace(currentLine[colonIdx+1:])
				if val != "" {
					for _, key := range ctx.Path {
						if key == "steps" {
							r := valueRange(pos.Line, currentLine, val)
							return []string{"steps", val}, &r
						}
					}
				}
			}
		}
	}

	return nil, nil
}

// extractKeyFromLine extracts the YAML key from a line like "  myKey: value" or "  - myKey: value".
func extractKeyFromLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- ") {
		trimmed = strings.TrimPrefix(trimmed, "- ")
	}
	colonIdx := strings.Index(trimmed, ":")
	if colonIdx < 0 {
		return ""
	}
	return strings.TrimSpace(trimmed[:colonIdx])
}

// extractBlockKeys extracts top-level keys under the given block name (e.g. "vars", "secrets").
func extractBlockKeys(text, blockName string) []string {
	lines := strings.Split(text, "\n")
	var keys []string
	inBlock := false
	blockIndent := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == blockName+":" {
			inBlock = true
			blockIndent = getLineIndent(line)
			continue
		}
		if inBlock {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			lineIndent := getLineIndent(line)
			if lineIndent <= blockIndent {
				break
			}
			key := extractKeyFromLine(line)
			if key != "" {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

// bindKeyInfo represents a key found in a bind.vars or bind.secrets block, with its line number.
type bindKeyInfo struct {
	key  string
	line int
}

// extractBindKeys scans the document for all bind.vars or bind.secrets keys.
// blockName should be "vars" or "secrets".
// Returns keys in document order (top to bottom).
func extractBindKeys(text, blockName string) []bindKeyInfo {
	lines := strings.Split(text, "\n")
	var result []bindKeyInfo

	// State machine: look for "bind:" → blockName+":" → keys.
	inBind := false
	bindIndent := -1
	inTargetBlock := false
	targetBlockIndent := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		lineIndent := getLineIndent(line)

		// Check if we left the bind block.
		if inBind && lineIndent <= bindIndent {
			inBind = false
			inTargetBlock = false
		}

		// Check if we left the target block (vars/secrets under bind).
		if inTargetBlock && lineIndent <= targetBlockIndent {
			inTargetBlock = false
		}

		if inTargetBlock {
			key := extractKeyFromLine(line)
			if key != "" {
				result = append(result, bindKeyInfo{key: key, line: i})
			}
			continue
		}

		if inBind && trimmed == blockName+":" {
			inTargetBlock = true
			targetBlockIndent = lineIndent
			continue
		}

		// Detect "bind:" (could appear at any step level).
		if trimmed == "bind:" {
			inBind = true
			bindIndent = lineIndent
			inTargetBlock = false
			continue
		}
		// Also handle "- bind:" at a list item start.
		stripped := strings.TrimPrefix(trimmed, "- ")
		if stripped == "bind:" && stripped != trimmed {
			inBind = true
			bindIndent = lineIndent + 2 // indent past the "- "
			inTargetBlock = false
			continue
		}
	}
	return result
}

// extractStepIDs extracts step IDs from the "steps" block of a document.
func extractStepIDs(text string) []string {
	lines := strings.Split(text, "\n")
	var ids []string
	inSteps := false
	stepsIndent := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "steps:" {
			inSteps = true
			stepsIndent = getLineIndent(line)
			continue
		}
		if inSteps {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			lineIndent := getLineIndent(line)
			if lineIndent <= stepsIndent {
				break
			}
			// Look for "id: <value>" lines.
			key := extractKeyFromLine(line)
			if key == "id" {
				colonIdx := strings.Index(trimmed, ":")
				if colonIdx >= 0 {
					val := strings.TrimSpace(trimmed[colonIdx+1:])
					// Remove surrounding quotes if present.
					val = strings.Trim(val, `"'`)
					if val != "" {
						ids = append(ids, val)
					}
				}
			}
		}
	}
	return ids
}

// configBlockKeys reads keys from the given block in the config file (scenarigo.yaml).
func (s *Server) configBlockKeys(blockName string) []string {
	configText, _, ok := s.readConfigText()
	if !ok {
		return nil
	}
	return extractBlockKeys(configText, blockName)
}

// keyRange returns a Range covering the key name on the given line.
func keyRange(line int, lineText, keyName string) Range {
	idx := strings.Index(lineText, keyName)
	if idx < 0 {
		return Range{Start: Position{Line: line, Character: 0}, End: Position{Line: line, Character: 0}}
	}
	return Range{
		Start: Position{Line: line, Character: idx},
		End:   Position{Line: line, Character: idx + len(keyName)},
	}
}

// valueRange returns a Range covering the value on the given line (after ":").
func valueRange(line int, lineText, value string) Range {
	colonIdx := strings.Index(lineText, ":")
	if colonIdx < 0 {
		return Range{Start: Position{Line: line, Character: 0}, End: Position{Line: line, Character: 0}}
	}
	// Find value after colon.
	afterColon := lineText[colonIdx+1:]
	valIdx := strings.Index(afterColon, value)
	if valIdx < 0 {
		return Range{Start: Position{Line: line, Character: 0}, End: Position{Line: line, Character: 0}}
	}
	start := colonIdx + 1 + valIdx
	return Range{
		Start: Position{Line: line, Character: start},
		End:   Position{Line: line, Character: start + len(value)},
	}
}

func (s *Server) references(doc *document, params ReferenceParams) []Location {
	if schema.DetectSchemaType(doc.Text) == nil {
		return nil
	}

	symbolPath, declRange := identifySymbol(doc, params.Position)
	if symbolPath == nil || len(symbolPath) < 2 {
		return nil
	}

	refs := scanTemplateRefs(doc.Text)

	var locs []Location

	// If includeDeclaration, add the declaration location.
	if params.Context.IncludeDeclaration && declRange != nil {
		locs = append(locs, Location{
			URI:   params.TextDocument.URI,
			Range: *declRange,
		})
	}

	// Match template refs whose first 2 path elements match symbolPath.
	for _, ref := range refs {
		if len(ref.path) < 2 {
			continue
		}
		if ref.path[0] == symbolPath[0] && ref.path[1] == symbolPath[1] {
			// Highlight just the matched prefix (e.g., "vars.myVar" portion).
			matchEnd := ref.startCol + len(ref.path[0]) + 1 + len(ref.path[1])
			if matchEnd > ref.endCol {
				matchEnd = ref.endCol
			}
			locs = append(locs, Location{
				URI: params.TextDocument.URI,
				Range: Range{
					Start: Position{Line: ref.line, Character: ref.startCol},
					End:   Position{Line: ref.line, Character: matchEnd},
				},
			})
		}
	}

	return locs
}

// findDeclRange searches the document for the declaration of a symbol and returns its range.
// For vars/secrets, it uses last-write-wins: the last definition (top-level or bind) wins.
func findDeclRange(doc *document, symbolPath []string) *Range {
	if len(symbolPath) < 2 || doc.Parsed == nil {
		return nil
	}

	root := symbolPath[0]
	name := symbolPath[1]
	lines := strings.Split(doc.Text, "\n")

	switch root {
	case "vars", "secrets":
		// Last-write-wins: collect all definitions (top-level + bind), return the last one.
		var lastRange *Range

		// Scan top-level block.
		if r := findBlockKeyRange(doc.Text, root, name); r != nil {
			lastRange = r
		}

		// Scan bind blocks — these appear later and override top-level.
		for _, bk := range extractBindKeys(doc.Text, root) {
			if bk.key == name {
				r := keyRange(bk.line, lines[bk.line], name)
				lastRange = &r
			}
		}

		return lastRange
	case "steps":
		// Find the step with id: <name>.
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Look for "id: <name>" or "- id: <name>".
			s := trimmed
			if strings.HasPrefix(s, "- ") {
				s = strings.TrimPrefix(s, "- ")
			}
			if strings.HasPrefix(s, "id:") {
				val := strings.TrimSpace(strings.TrimPrefix(s, "id:"))
				if val == name {
					r := valueRange(i, line, name)
					return &r
				}
			}
		}
	}
	return nil
}

func getLineIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// getFullTemplateExpr returns the full template expression surrounding the cursor.
// Unlike getTemplateContext which returns text up to the cursor, this returns the entire expression.
func getFullTemplateExpr(text string, pos Position) (string, bool) {
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
	// Find the closing "}}" after the opening.
	closeIdx := strings.Index(line[openIdx+2:], "}}")
	if closeIdx < 0 {
		return "", false
	}
	return strings.TrimSpace(line[openIdx+2 : openIdx+2+closeIdx]), true
}
