package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/goccy/go-yaml/ast"

	"github.com/scenarigo/scenarigo/internal/yamlschema"
)

// Server is the LSP server.
type Server struct {
	reader            io.Reader
	writer            io.Writer
	logger            *log.Logger
	docs              *documentStore
	rootURI           string
	encoding          positionEncoding // unit of Position.Character on the wire, see position.go
	writeMu           sync.Mutex       // serializes writeMessage
	shutdownRequested bool
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
