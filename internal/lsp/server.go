package lsp

import (
	"encoding/json"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/goccy/go-yaml/ast"

	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
	"github.com/scenarigo/scenarigo/internal/yamlschema"
)

// Template namespaces, which are also the YAML blocks that declare them.
const (
	nsVars    = "vars"
	nsSecrets = "secrets"
	nsSteps   = "steps"
	nsPlugins = "plugins"
)

// Server is the LSP server.
type Server struct {
	reader             io.Reader
	writer             io.Writer
	logger             *log.Logger
	docs               *documentStore
	rootURI            string
	encoding           positionEncoding // unit of Position.Character on the wire, see position.go
	writeMu            sync.Mutex       // serializes writeMessage
	shutdownRequested  bool
	pluginSymbolsMu    sync.Mutex
	pluginSymbolsCache map[string]*pluginSymbols // source dir → exported symbols
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
	case "textDocument/completion":
		s.handleCompletion(req)
	case "textDocument/hover":
		s.handleHover(req)
	case "textDocument/signatureHelp":
		s.handleSignatureHelp(req)
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
			CompletionProvider: &CompletionOptions{
				TriggerCharacters: append([]string{":", " ", "\n"}, templateTriggerCharacters...),
			},
			HoverProvider: true,
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
		s.sendResponse(req.ID, nil, invalidParams("completion", err))
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(req.ID, CompletionList{Items: []CompletionItem{}}, nil)
		return
	}

	pos := s.decodePosition(doc.Text, params.Position)
	if isTemplateTrigger(params.Context) {
		if _, ok := getTemplateContext(doc.Text, pos); !ok {
			// Clients replace only the text after "." or "{", so a file path or
			// value accepted here would be garbled ("steps." + "steps.yaml").
			s.sendResponse(req.ID, CompletionList{Items: []CompletionItem{}}, nil)
			return
		}
	}
	items := s.complete(doc, pos)
	for i := range items {
		if items[i].TextEdit != nil {
			items[i].TextEdit.Range = s.encodeRange(doc.Text, items[i].TextEdit.Range)
		}
	}
	if items == nil {
		items = []CompletionItem{}
	}
	s.sendResponse(req.ID, CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil)
}

// templateTriggerCharacters start and continue template expressions such as
// {{vars.token}}. They trigger completion inside "{{" only.
var templateTriggerCharacters = []string{"{", "."}

// isTemplateTrigger reports whether completion was triggered by typing one of
// templateTriggerCharacters.
func isTemplateTrigger(c *CompletionContext) bool {
	return c != nil && c.TriggerKind == CompletionTriggerKindTriggerCharacter &&
		slices.Contains(templateTriggerCharacters, c.TriggerCharacter)
}

func (s *Server) handleHover(req *Request) {
	var params HoverParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendResponse(req.ID, nil, invalidParams("hover", err))
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}

	hover := s.hover(doc, s.decodePosition(doc.Text, params.Position))
	if hover == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}
	s.sendResponse(req.ID, hover, nil)
}

func (s *Server) handleSignatureHelp(req *Request) {
	var params SignatureHelpParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendResponse(req.ID, nil, invalidParams("signatureHelp", err))
		return
	}

	doc := s.docs.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(req.ID, nil, nil)
		return
	}

	help := s.signatureHelp(doc, s.decodePosition(doc.Text, params.Position))
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

// readConfigText returns the text of scenarigo.yaml and its URI.
// It checks the document store first, then falls back to reading from disk.
func (s *Server) readConfigText() (string, string, bool) {
	rootPath := uriToPath(s.rootURI)
	if rootPath == "" {
		return "", "", false
	}
	configPath := filepath.Join(rootPath, "scenarigo.yaml")
	if doc := s.docs.GetByPath(configPath); doc != nil {
		return doc.Text, doc.URI, true
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", "", false
	}
	return string(data), pathToURI(configPath), true
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

// parseGoPackageFiles parses the non-test Go files in dir one by one.
// parser.ParseDir is deprecated together with ast.Package.
func parseGoPackageFiles(fset *token.FileSet, dir string, mode parser.Mode) ([]*goast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []*goast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, mode)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// extractGoExportedSymbols parses Go source files in dir and returns exported symbols with signatures and docs.
func extractGoExportedSymbols(dir string) (*pluginSymbols, error) {
	fset := token.NewFileSet()
	files, err := parseGoPackageFiles(fset, dir, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var syms pluginSymbols
	for _, file := range files {
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
// Flow: scenario "plugins: { grpc: grpc.so }" → config "plugins: { grpc.so: { src: ./plugin/src } }".
func (s *Server) resolvePluginSourceDir(doc *document, pluginAlias string) string {
	rootPath := uriToPath(s.rootURI)
	if rootPath == "" {
		return ""
	}

	// Get the binary name from the scenario's plugins block.
	binaryName := ""
	if doc != nil {
		for _, key := range extractBlockKeys(doc.Text, nsPlugins) {
			if key == pluginAlias {
				binaryName = extractBlockValue(doc.Text, nsPlugins, key)
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
				_, after, ok := strings.Cut(trimmed, ":")
				if ok {
					val := strings.TrimSpace(after)
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
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+":") {
			val := strings.TrimSpace(trimmed[len(key)+1:])
			return strings.Trim(val, `"'`)
		}
	}
	return ""
}

// extractPluginSrc extracts the src value for a plugin binary from config text.
// Config format: "plugins:\n  binary.so:\n    src: ./path".
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
				_, after, ok := strings.Cut(trimmed, ":")
				if ok {
					val := strings.TrimSpace(after)
					val = strings.Trim(val, `"'`)
					return val
				}
			}
		}
	}
	return ""
}

func (s *Server) complete(doc *document, pos Position) []CompletionItem {
	sch := yamlschema.DetectSchemaType(doc.Text)
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
		{name: "ctx", detail: "Execution context (for plugin functions)", kind: CompletionItemKindVariable},
		{name: nsVars, detail: "Scenario/step variables", kind: CompletionItemKindVariable},
		{name: nsSecrets, detail: "Secret variables", kind: CompletionItemKindVariable},
		{name: nsPlugins, detail: "Plugin exports", kind: CompletionItemKindVariable},
		{name: "request", detail: "Request data (protocol-specific)", kind: CompletionItemKindVariable},
		{name: "response", detail: "Response data (protocol-specific)", kind: CompletionItemKindVariable},
		{name: nsSteps, detail: "Results from previous steps", kind: CompletionItemKindVariable},
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

// assertCandidates lists the assertion helpers exposed under {{assert.}}.
func assertCandidates() []templateCandidate {
	return []templateCandidate{
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
}

// pluginSymbolCandidates lists the exported symbols of a plugin for {{plugins.<alias>.}}.
func (s *Server) pluginSymbolCandidates(doc *document, alias string) []templateCandidate {
	sourceDir := s.resolvePluginSourceDir(doc, alias)
	if sourceDir == "" {
		return nil
	}
	syms := s.getPluginSymbols(sourceDir)
	if syms == nil {
		return nil
	}
	var candidates []templateCandidate
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
	return candidates
}

func (s *Server) completeTemplateDot(doc *document, prefix, partial string) []CompletionItem {
	// Known sub-completions.
	var candidates []templateCandidate

	switch prefix {
	case "assert":
		candidates = assertCandidates()
	case nsVars, nsSecrets:
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
	case nsSteps:
		if doc != nil {
			for _, id := range extractStepIDs(doc.Text) {
				candidates = append(candidates, templateCandidate{name: id, detail: "Step result", kind: CompletionItemKindVariable})
			}
		}
	case nsPlugins:
		if doc != nil {
			for _, name := range extractBlockKeys(doc.Text, nsPlugins) {
				candidates = append(candidates, templateCandidate{name: name, detail: "Plugin", kind: CompletionItemKindModule})
			}
		}
		// Also look for plugins defined in the config file.
		for _, name := range s.configBlockKeys(nsPlugins) {
			candidates = append(candidates, templateCandidate{name: name, detail: "Plugin (from scenarigo.yaml)", kind: CompletionItemKindModule})
		}
	default:
		// Handle plugins.<name> — complete exported symbols from plugin source.
		if alias, ok := strings.CutPrefix(prefix, "plugins."); ok {
			candidates = s.pluginSymbolCandidates(doc, alias)
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

func (s *Server) completeKeys(sch *yamlschema.Schema, ctx *yamlutil.CursorContext, pos Position) []CompletionItem {
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
		switch f.Type {
		case yamlschema.FieldTypeObject:
			insertText = f.Name + ":"
		case yamlschema.FieldTypeArray:
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

func (s *Server) completeValues(sch *yamlschema.Schema, ctx *yamlutil.CursorContext, docURI string) []CompletionItem {
	if len(ctx.Path) == 0 {
		return nil
	}

	field := sch.FindField(ctx.Path)
	if field == nil {
		// For map-type fields like plugins, the full path (e.g., ["plugins", "myplugin"])
		// won't match a schema field. Check if the parent is a file-path map field.
		if len(ctx.Path) >= 2 {
			parentField := sch.FindField(ctx.Path[:len(ctx.Path)-1])
			if parentField != nil && parentField.Type == yamlschema.FieldTypeMap && parentField.IsFilePath {
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
	if field.Type == yamlschema.FieldTypeBool {
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
func (s *Server) completeFilePathFromContext(sch *yamlschema.Schema, ctx *yamlutil.CursorContext, docURI string) []CompletionItem {
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

func (s *Server) hover(doc *document, pos Position) *Hover {
	sch := yamlschema.DetectSchemaType(doc.Text)
	if sch == nil {
		return nil
	}
	if doc.Parsed == nil {
		return nil
	}

	// Find node at position (convert from 0-based to 1-based).
	nodePath := doc.Parsed.FindNodeAtPosition(pos.Line+1, runeColumnFromByte(lineAt(doc.Text, pos.Line), pos.Character)+1)
	if nodePath == nil || len(nodePath.Keys) == 0 {
		return nil
	}

	field := sch.FindField(nodePath.Keys)
	if field == nil {
		return nil
	}

	content := fmt.Sprintf("**%s** (`%s`)\n\n%s", field.Name, field.Type, field.Description)
	if len(field.EnumValues) > 0 {
		label := "Allowed values"
		if field.OpenEnum {
			label = "Built-in values"
		}
		content += fmt.Sprintf("\n\n%s: `%s`", label, strings.Join(field.EnumValues, "`, `"))
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

	// A cached AST is of an earlier text: its positions do not match.
	broken := doc.Parsed == nil || doc.Parsed.Text != doc.Text
	switch {
	case !isScenarigoDocument(doc):
		// Not a scenarigo YAML file; send empty diagnostics and stay silent.
	case broken && strings.Contains(doc.Text, "#@"):
		// A ytt template, whose branches can repeat a key, is YAML only after
		// ytt evaluates it.
	case broken:
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 0},
			},
			Severity: DiagnosticSeverityError,
			Message:  "Invalid YAML syntax",
		})
	default:
		diagnostics = append(diagnostics, s.validateDocument(doc)...)
	}

	for i := range diagnostics {
		diagnostics[i].Range = s.encodeRange(doc.Text, diagnostics[i].Range)
	}
	s.sendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	})
}

func (s *Server) validateDocument(doc *document) []Diagnostic {
	text := doc.Text
	if doc.Parsed == nil || doc.Parsed.File == nil {
		return nil
	}
	var diags []Diagnostic
	for v, d := range yamlDocuments(doc.Parsed.File) {
		if d.Body == nil {
			continue
		}
		// A file may mix documents, for example ytt/v1 and scenario/v1.
		s.validateNode(text, v, d.Body, documentFields(v, d.Body), nil, &diags)
	}
	return diags
}

func (s *Server) validateNode(text string, v *yamlView, node ast.Node, fields []*yamlschema.FieldInfo, sibling func(key string) string, diags *[]Diagnostic) {
	if node == nil || fields == nil {
		return
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		// Expanded when a field asks: many mappings can merge a large one.
		var entries []*ast.MappingValueNode
		sibling := func(key string) string {
			if entries == nil {
				entries = v.entries(n)
			}
			return v.stringValue(v.entryValue(entries, key))
		}
		// Validate each key. The merged ones are not validated here.
		presentKeys := make(map[string]bool)
		merges := false
		for _, mv := range n.Values {
			if _, ok := mv.Key.(*ast.MergeKeyNode); ok {
				merges = true
				continue
			}
			if mv.Key != nil {
				presentKeys[keyName(mv.Key)] = true
			}
			s.validateMappingValue(text, v, mv, fields, sibling, diags)
		}
		// Check required fields, unless a merge key may bring them.
		if !merges {
			s.validateRequiredFields(text, n, fields, presentKeys, diags)
		}
	case *ast.MappingValueNode:
		s.validateMappingValue(text, v, n, fields, sibling, diags)
	}
}

func (s *Server) validateMappingValue(text string, view *yamlView, mv *ast.MappingValueNode, fields []*yamlschema.FieldInfo, sibling func(key string) string, diags *[]Diagnostic) {
	if mv.Key == nil {
		return
	}
	name := keyName(mv.Key)
	tok := mv.Key.GetToken()

	// Find matching field in schema.
	var field *yamlschema.FieldInfo
	for _, f := range fields {
		if f.Name == name {
			field = f
			break
		}
	}

	if field == nil {
		// Unknown key.
		if tok != nil {
			*diags = append(*diags, Diagnostic{
				Range:    tokenRange(text, tok.Position.Line, nameColumn(tok), len(name)),
				Severity: DiagnosticSeverityWarning,
				Message:  fmt.Sprintf("unknown field %q", name),
			})
		}
		return
	}

	validateEnumValue(text, mv.Value, field, name, diags)

	// Validate type.
	if mv.Value != nil && !yttFillsValue(text, mv) {
		s.validateFieldType(text, mv.Value, field, name, diags)
	}

	// Recurse into child nodes.
	if mv.Value != nil {
		var childFields []*yamlschema.FieldInfo
		if field.DynamicChildren != nil {
			discriminator := ""
			if field.DynamicKey != "" && sibling != nil {
				discriminator = sibling(field.DynamicKey)
			}
			childFields = field.DynamicChildren(discriminator)
		} else {
			childFields = field.Children
		}

		switch v := mv.Value.(type) {
		case *ast.MappingNode:
			if field.Type == yamlschema.FieldTypeMap {
				// Map keys are user-defined; validate each value against the children.
				for _, entry := range v.Values {
					if m, ok := entry.Value.(*ast.MappingNode); ok {
						s.validateNode(text, view, m, childFields, nil, diags)
					}
				}
			} else {
				s.validateNode(text, view, v, childFields, nil, diags)
			}
		case *ast.SequenceNode:
			// For sequences with object items (e.g., steps), validate each item.
			if field.Children != nil {
				for _, item := range v.Values {
					if m, ok := item.(*ast.MappingNode); ok {
						s.validateNode(text, view, m, field.Children, nil, diags)
					}
				}
			}
		}
	}
}

// validateEnumValue reports a value outside the enumerated values of field.
// An empty value selects the default in every closed enumeration scenarigo
// reads (schemaVersion, the HTTP method, the tracing exporter), so it is
// reported only for open ones such as protocol, which has no default.
func validateEnumValue(text string, value ast.Node, field *yamlschema.FieldInfo, keyName string, diags *[]Diagnostic) {
	if len(field.EnumValues) == 0 || value == nil {
		return
	}
	sv, ok := value.(*ast.StringNode)
	if !ok || (sv.Value == "" && (!field.OpenEnum || field.AllowEmpty)) || slices.Contains(field.EnumValues, sv.Value) {
		return
	}
	valTok := value.GetToken()
	if valTok == nil {
		return
	}
	diag := Diagnostic{
		Range:    tokenRange(text, valTok.Position.Line, valTok.Position.Column, len(sv.Value)),
		Severity: DiagnosticSeverityWarning,
		Message:  fmt.Sprintf("invalid value %q for field %q (allowed: %s)", sv.Value, keyName, strings.Join(field.EnumValues, ", ")),
	}
	if field.OpenEnum {
		// Other values are valid too, so only hint at a possible typo.
		diag.Severity = DiagnosticSeverityInformation
		diag.Message = fmt.Sprintf("%q is not a built-in value for field %q (built-in: %s)", sv.Value, keyName, strings.Join(field.EnumValues, ", "))
	}
	*diags = append(*diags, diag)
}

func (s *Server) validateRequiredFields(text string, node *ast.MappingNode, fields []*yamlschema.FieldInfo, presentKeys map[string]bool, diags *[]Diagnostic) {
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
			Range:    tokenRange(text, tok.Position.Line, tok.Position.Column, 0),
			Severity: DiagnosticSeverityWarning,
			Message:  fmt.Sprintf("missing required field %q", f.Name),
		})
	}
}

// Labels of the YAML node types used in type diagnostics.
const (
	nodeTypeBool    = "bool"
	nodeTypeInt     = "int"
	nodeTypeFloat   = "float"
	nodeTypeString  = "string"
	nodeTypeObject  = "object"
	nodeTypeArray   = "array"
	nodeTypeNull    = "null"
	nodeTypeUnknown = "unknown"
)

// acceptedNodeTypes lists the YAML node types a field type accepts. Strings
// are accepted everywhere because they may hold template expressions, and
// scalars are accepted as strings.
var acceptedNodeTypes = map[yamlschema.FieldType][]string{
	yamlschema.FieldTypeBool:     {nodeTypeBool, nodeTypeString},
	yamlschema.FieldTypeString:   {nodeTypeString, nodeTypeInt, nodeTypeFloat, nodeTypeBool},
	yamlschema.FieldTypeDuration: {nodeTypeString, nodeTypeInt, nodeTypeFloat, nodeTypeBool},
	yamlschema.FieldTypeInt:      {nodeTypeInt, nodeTypeString},
	yamlschema.FieldTypeFloat:    {nodeTypeInt, nodeTypeFloat, nodeTypeString},
	yamlschema.FieldTypeObject:   {nodeTypeObject, nodeTypeString, nodeTypeNull},
	yamlschema.FieldTypeArray:    {nodeTypeArray, nodeTypeString, nodeTypeNull},
}

func (s *Server) validateFieldType(text string, value ast.Node, field *yamlschema.FieldInfo, keyName string, diags *[]Diagnostic) {
	accepted, ok := acceptedNodeTypes[field.Type]
	if !ok {
		return // Any and Map accept anything.
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
	case *ast.TagNode:
		return // The tag decides the type: !!str 1 is a string.
	case *ast.AnchorNode:
		if v.Value != nil {
			s.validateFieldType(text, v.Value, field, keyName, diags)
		}
		return
	}

	got := describeNodeType(value)
	if slices.Contains(accepted, got) {
		return
	}
	tok := value.GetToken()
	if tok == nil {
		return
	}
	*diags = append(*diags, Diagnostic{
		Range:    tokenRange(text, tok.Position.Line, tok.Position.Column, len(value.String())),
		Severity: DiagnosticSeverityWarning,
		Message:  fmt.Sprintf("field %q expects %s, got %s", keyName, field.Type, got),
	})
}

func describeNodeType(node ast.Node) string {
	switch node.(type) {
	case *ast.BoolNode:
		return nodeTypeBool
	case *ast.IntegerNode:
		return nodeTypeInt
	case *ast.FloatNode:
		return nodeTypeFloat
	case *ast.StringNode, *ast.LiteralNode:
		return nodeTypeString
	case *ast.MappingNode:
		return nodeTypeObject
	case *ast.SequenceNode:
		return nodeTypeArray
	case *ast.NullNode:
		return nodeTypeNull
	default:
		return nodeTypeUnknown
	}
}

// extractKeyFromLine extracts the YAML key from a line like "  myKey: value" or "  - myKey: value".
func extractKeyFromLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if after, ok := strings.CutPrefix(trimmed, "- "); ok {
		trimmed = after
	}
	before, _, ok := strings.Cut(trimmed, ":")
	if !ok {
		return ""
	}
	return strings.TrimSpace(before)
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
				_, after, ok := strings.Cut(trimmed, ":")
				if ok {
					val := strings.TrimSpace(after)
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

func getLineIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}
