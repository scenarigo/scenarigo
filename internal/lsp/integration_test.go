package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestEditorSession_OpenEditComplete(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)

	client.initialize(1, root)

	// Open a document.
	docText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    "
	client.openDocument(file("test.yaml"), docText)

	// Get completions.
	list := client.complete(2, file("test.yaml"), 5, 4)
	labels := labelSet(list.Items)
	if !labels["request"] {
		t.Errorf("expected 'request' in initial completions, got: %v", labelList(list.Items))
	}

	// Edit the document (add request key).
	newText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    request:\n      "
	client.changeDocument(file("test.yaml"), 2, newText)

	// Completions should now show request fields.
	list = client.complete(3, file("test.yaml"), 6, 6)
	labels = labelSet(list.Items)
	if !labels["method"] {
		t.Errorf("expected 'method' after edit, got: %v", labelList(list.Items))
	}
}

func TestEditorSession_MultipleDocuments(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)

	client.initialize(1, root)

	// Open a scenario file.
	scenarioText := "schemaVersion: scenario/v1\ntitle: test\n"
	client.openDocument(file("scenario.yaml"), scenarioText)

	// Open a config file.
	configText := "schemaVersion: config/v1\n"
	client.openDocument(file("config.yaml"), configText)

	// Complete in scenario file.
	list1 := client.complete(2, file("scenario.yaml"), 2, 0)
	labels1 := labelSet(list1.Items)
	if !labels1["steps"] {
		t.Errorf("expected 'steps' in scenario completions, got: %v", labelList(list1.Items))
	}

	// Complete in config file.
	list2 := client.complete(3, file("config.yaml"), 1, 0)
	labels2 := labelSet(list2.Items)
	if !labels2["scenarios"] {
		t.Errorf("expected 'scenarios' in config completions, got: %v", labelList(list2.Items))
	}
}

func TestEditorSession_EditAndDiagnostics(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)

	client.initialize(1, root)

	// Open valid document.
	docText := "schemaVersion: scenario/v1\ntitle: test\n"
	diags := client.openDocumentAndGetDiagnostics(file("test.yaml"), docText)
	if len(diags.Diagnostics) > 0 {
		t.Errorf("expected no diagnostics for valid document, got: %v", diagMessages(diags.Diagnostics))
	}

	// Edit to introduce unknown key.
	badText := "schemaVersion: scenario/v1\ntitle: test\nbadKey: value\n"
	client.sendNotification("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: TextDocumentIdentifier{URI: file("test.yaml")},
			Version:                2,
		},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: badText}},
	})
	// Wait and read diagnostics manually (not using changeDocument helper since we need raw notification).
	raw := client.readMessage()
	var notif Notification
	if err := json.Unmarshal(raw, &notif); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	var updatedDiags PublishDiagnosticsParams
	if err := json.Unmarshal(notif.Params, &updatedDiags); err != nil {
		t.Fatalf("unmarshal diagnostics: %v", err)
	}

	found := false
	for _, d := range updatedDiags.Diagnostics {
		if d.Message == `unknown field "badKey"` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic for badKey after edit, got: %v", diagMessages(updatedDiags.Diagnostics))
	}
}

func TestEditorSession_CodeAction_DidYouMean(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)

	client.initialize(1, root)

	// Open document with a typo: "protocl" instead of "protocol".
	//nolint:misspell // the typo is the point of this test
	docText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocl: http\n"
	diags := client.openDocumentAndGetDiagnostics(file("test.yaml"), docText)

	// Find the diagnostic for the unknown field.
	var unknownDiag *Diagnostic
	for i, d := range diags.Diagnostics {
		if d.Message == `unknown field "protocl"` { //nolint:misspell // see docText
			unknownDiag = &diags.Diagnostics[i]
			break
		}
	}
	if unknownDiag == nil {
		t.Fatalf("expected diagnostic for protocl, got: %v", diagMessages(diags.Diagnostics)) //nolint:misspell // see docText
	}

	// Request code actions with the diagnostic.
	resp := client.codeAction(2, file("test.yaml"), unknownDiag.Range, []Diagnostic{*unknownDiag})

	var actions []CodeAction
	if err := json.Unmarshal(resp, &actions); err != nil {
		t.Fatalf("unmarshal code actions: %v", err)
	}

	// Should suggest "protocol".
	found := false
	for _, a := range actions {
		if a.Title == `Did you mean "protocol"?` {
			found = true
			break
		}
	}
	if !found {
		titles := make([]string, len(actions))
		for i, a := range actions {
			titles[i] = a.Title
		}
		t.Errorf("expected 'Did you mean \"protocol\"?' in actions, got: %v", titles)
	}
}

func TestEditorSession_ForeignModelineSkipped(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)

	client.initialize(1, root)

	// Open a file with a yaml-language-server modeline.
	// The server should NOT store it and NOT send diagnostics.
	foreignText := "# yaml-language-server: $schema=https://json.schemastore.org/github-workflow\nname: CI\non: push\n"
	client.sendNotification("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        file("workflow.yaml"),
			LanguageID: "yaml",
			Version:    1,
			Text:       foreignText,
		},
	})

	// No diagnostics notification should be sent.
	// Verify by requesting completion — should return empty since doc is not in store.
	list := client.complete(2, file("workflow.yaml"), 2, 0)
	if len(list.Items) != 0 {
		t.Errorf("expected no completions for foreign modeline file, got: %v", labelList(list.Items))
	}
}

// TestEditorSession_FullWorkflow simulates a realistic editor workflow:
// initialize → open → diagnostics → hover → symbols → references → close → shutdown.
func TestEditorSession_FullWorkflow(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)

	// Initialize and verify capabilities.
	initResult := client.initialize(1, root)
	if initResult.Capabilities.CompletionProvider == nil {
		t.Fatal("expected completion provider")
	}
	if !initResult.Capabilities.HoverProvider {
		t.Error("expected hover provider")
	}
	if !initResult.Capabilities.DefinitionProvider {
		t.Error("expected definition provider")
	}
	if !initResult.Capabilities.DocumentSymbolProvider {
		t.Error("expected document symbol provider")
	}
	if !initResult.Capabilities.CodeActionProvider {
		t.Error("expected code action provider")
	}
	if !initResult.Capabilities.ReferencesProvider {
		t.Error("expected references provider")
	}

	// Open a scenario with vars and template references.
	docText := "schemaVersion: scenario/v1\ntitle: integration test\nvars:\n  token: abc123\nsteps:\n  - id: login\n    title: login\n    protocol: http\n    request:\n      method: POST\n      url: http://example.com/login\n      header:\n        Authorization: 'Bearer {{vars.token}}'\n"
	diags := client.openDocumentAndGetDiagnostics(file("test.yaml"), docText)
	if len(diags.Diagnostics) != 0 {
		t.Errorf("expected no diagnostics, got: %v", diagMessages(diags.Diagnostics))
	}

	// Completion.
	list := client.complete(2, file("test.yaml"), 12, 8)
	_ = list // verify no panic

	// Hover on "protocol".
	hoverResp := client.hover(3, file("test.yaml"), 7, 6)
	var hoverResult Hover
	if err := json.Unmarshal(hoverResp, &hoverResult); err != nil {
		t.Fatalf("unmarshal hover: %v", err)
	}
	if hoverResult.Contents.Value == "" {
		t.Error("expected non-empty hover content for 'protocol'")
	}

	// Document symbols.
	symResp := client.documentSymbol(4, file("test.yaml"))
	var symbols []DocumentSymbol
	if err := json.Unmarshal(symResp, &symbols); err != nil {
		t.Fatalf("unmarshal symbols: %v", err)
	}
	names := collectSymbolNames(symbols)
	if !names["title"] || !names["vars"] || !names["steps"] {
		t.Errorf("expected title, vars, steps in symbols, got: %v", symbolNameList(symbols))
	}

	// References on "token" (line 3, char 4 = the key "token" under vars).
	refsResp := client.references(5, file("test.yaml"), 3, 4)
	var locs []Location
	if err := json.Unmarshal(refsResp, &locs); err != nil {
		t.Fatalf("unmarshal locations: %v", err)
	}
	if len(locs) < 2 {
		t.Errorf("expected at least 2 references (decl + usage), got %d", len(locs))
	}

	// Close document.
	client.closeDocument(file("test.yaml"))

	// Shutdown.
	client.shutdown(6)
}

// TestEditorSession_PluginExportCompletion verifies that plugin export completion
// includes signatures and doc comments from Go source.
func TestEditorSession_PluginExportCompletion(t *testing.T) {
	tmpDir := t.TempDir()

	// Write config file.
	configContent := "schemaVersion: config/v1\nplugins:\n  myplugin.so:\n    src: ./plugin/src\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "scenarigo.yaml"), []byte(configContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Write plugin Go source.
	srcDir := filepath.Join(tmpDir, "plugin", "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	goSrc := `package main

// CreateClient creates a new test client connected to the given address.
func CreateClient(ctx interface{}, addr string) interface{} { return nil }

// DefaultTimeout is the default timeout for requests.
var DefaultTimeout int
`
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(goSrc), 0o600); err != nil {
		t.Fatalf("write go source: %v", err)
	}

	client := newRunningTestClient(t)

	rootURI := "file://" + tmpDir
	client.initialize(1, rootURI)

	// Open a scenario that uses {{plugins.myplugin.
	docText := "schemaVersion: scenario/v1\nplugins:\n  myplugin: myplugin.so\nsteps:\n  - title: step1\n    request:\n      client: '{{plugins.myplugin.'\n"
	uri := "file://" + filepath.Join(tmpDir, "test.yaml")
	client.openDocument(uri, docText)

	// Complete at the cursor position (line 6, after "myplugin.").
	list := client.complete(2, uri, 6, 34)

	// Verify labels.
	labels := labelSet(list.Items)
	if !labels["CreateClient"] {
		t.Errorf("expected CreateClient in completions, got: %v", labelList(list.Items))
	}
	if !labels["DefaultTimeout"] {
		t.Errorf("expected DefaultTimeout in completions, got: %v", labelList(list.Items))
	}

	// Verify signature and doc for CreateClient.
	for _, item := range list.Items {
		if item.Label == "CreateClient" {
			if item.Detail == "" {
				t.Error("expected Detail (signature) for CreateClient, got empty")
			}
			if item.Kind != CompletionItemKindFunction {
				t.Errorf("expected Function kind for CreateClient, got %d", item.Kind)
			}
			if item.Documentation == "" {
				t.Error("expected Documentation (doc comment) for CreateClient, got empty")
			}
			break
		}
	}

	// Verify DefaultTimeout is a variable.
	for _, item := range list.Items {
		if item.Label == "DefaultTimeout" {
			if item.Kind != CompletionItemKindVariable {
				t.Errorf("expected Variable kind for DefaultTimeout, got %d", item.Kind)
			}
			break
		}
	}
}

// TestEditorSession_PositionEncoding checks that columns after multibyte
// characters are exchanged in the negotiated unit: UTF-16 code units by
// default, bytes when the client offers utf-8.
func TestEditorSession_PositionEncoding(t *testing.T) {
	const (
		docText        = "schemaVersion: scenario/v1\nsteps:\n  - {title: 日本語, protocol: http, badKey: 1}\n"
		beforeProtocol = "  - {title: 日本語, "
		beforeBadKey   = beforeProtocol + "protocol: http, "
	)
	tests := []struct {
		name     string
		offered  []string
		encoding string
		col      func(prefix string) int
	}{
		{
			name:     "utf-16 by default",
			encoding: "utf-16",
			col:      func(prefix string) int { return len(utf16.Encode([]rune(prefix))) },
		},
		{
			name:     "utf-8 when offered",
			offered:  []string{"utf-8", "utf-16"},
			encoding: "utf-8",
			col:      func(prefix string) int { return len(prefix) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newRunningTestClient(t)
			root, file := newWorkspace(t)

			result := client.initializeWith(1, InitializeParams{
				RootURI:      root,
				Capabilities: ClientCapabilities{General: &GeneralClientCapabilities{PositionEncodings: tt.offered}},
			})
			if result.Capabilities.PositionEncoding != tt.encoding {
				t.Fatalf("positionEncoding = %q, want %q", result.Capabilities.PositionEncoding, tt.encoding)
			}

			uri := file("test.yaml")
			diags := client.openDocumentAndGetDiagnostics(uri, docText)
			var badKey *Diagnostic
			for i := range diags.Diagnostics {
				if strings.Contains(diags.Diagnostics[i].Message, "badKey") {
					badKey = &diags.Diagnostics[i]
				}
			}
			if badKey == nil {
				t.Fatalf("no diagnostic for badKey: %v", diagMessages(diags.Diagnostics))
			}
			want := Range{
				Start: Position{Line: 2, Character: tt.col(beforeBadKey)},
				End:   Position{Line: 2, Character: tt.col(beforeBadKey) + len("badKey")},
			}
			if badKey.Range != want {
				t.Errorf("badKey range = %+v, want %+v", badKey.Range, want)
			}

			hoverResp := client.hover(2, uri, 2, tt.col(beforeProtocol))
			if !strings.Contains(string(hoverResp), "**protocol**") {
				t.Errorf("hover at the protocol key = %s", hoverResp)
			}
		})
	}
}
