package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestEditorSession_OpenEditComplete(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	// Open a document.
	docText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    "
	client.openDocument("file:///tmp/test.yaml", docText)

	// Get completions.
	list := client.complete(2, "file:///tmp/test.yaml", 5, 4)
	labels := labelSet(list.Items)
	if !labels["request"] {
		t.Errorf("expected 'request' in initial completions, got: %v", labelList(list.Items))
	}

	// Edit the document (add request key).
	newText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    request:\n      "
	client.changeDocument("file:///tmp/test.yaml", 2, newText)

	// Completions should now show request fields.
	list = client.complete(3, "file:///tmp/test.yaml", 6, 6)
	labels = labelSet(list.Items)
	if !labels["method"] {
		t.Errorf("expected 'method' after edit, got: %v", labelList(list.Items))
	}
}

func TestEditorSession_MultipleDocuments(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	// Open a scenario file.
	scenarioText := "schemaVersion: scenario/v1\ntitle: test\n"
	client.openDocument("file:///tmp/scenario.yaml", scenarioText)

	// Open a config file.
	configText := "schemaVersion: config/v1\n"
	client.openDocument("file:///tmp/config.yaml", configText)

	// Complete in scenario file.
	list1 := client.complete(2, "file:///tmp/scenario.yaml", 2, 0)
	labels1 := labelSet(list1.Items)
	if !labels1["steps"] {
		t.Errorf("expected 'steps' in scenario completions, got: %v", labelList(list1.Items))
	}

	// Complete in config file.
	list2 := client.complete(3, "file:///tmp/config.yaml", 1, 0)
	labels2 := labelSet(list2.Items)
	if !labels2["scenarios"] {
		t.Errorf("expected 'scenarios' in config completions, got: %v", labelList(list2.Items))
	}
}

func TestEditorSession_EditAndDiagnostics(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	// Open valid document.
	docText := "schemaVersion: scenario/v1\ntitle: test\n"
	diags := client.openDocumentAndGetDiagnostics("file:///tmp/test.yaml", docText)
	if len(diags.Diagnostics) > 0 {
		t.Errorf("expected no diagnostics for valid document, got: %v", diagMessages(diags.Diagnostics))
	}

	// Edit to introduce unknown key.
	badText := "schemaVersion: scenario/v1\ntitle: test\nbadKey: value\n"
	client.sendNotification("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: TextDocumentIdentifier{URI: "file:///tmp/test.yaml"},
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

func TestEditorSession_Lifecycle(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	// Initialize.
	initResult := client.initialize(1, "file:///tmp")
	if initResult.Capabilities.CompletionProvider == nil {
		t.Fatal("expected completion provider")
	}

	// Open document.
	docText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    request:\n      method: GET\n      url: http://example.com\n"
	client.openDocument("file:///tmp/test.yaml", docText)

	// Completion.
	list := client.complete(2, "file:///tmp/test.yaml", 8, 6)
	_ = list // just verify no panic

	// Hover.
	client.hover(3, "file:///tmp/test.yaml", 4, 5)

	// Close document.
	client.closeDocument("file:///tmp/test.yaml")

	// Shutdown.
	client.shutdown(4)
}

func TestEditorSession_CodeAction_DidYouMean(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	// Open document with a typo: "protocl" instead of "protocol".
	docText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocl: http\n"
	diags := client.openDocumentAndGetDiagnostics("file:///tmp/test.yaml", docText)

	// Find the diagnostic for the unknown field.
	var unknownDiag *Diagnostic
	for i, d := range diags.Diagnostics {
		if d.Message == `unknown field "protocl"` {
			unknownDiag = &diags.Diagnostics[i]
			break
		}
	}
	if unknownDiag == nil {
		t.Fatalf("expected diagnostic for protocl, got: %v", diagMessages(diags.Diagnostics))
	}

	// Request code actions with the diagnostic.
	resp := client.codeAction(2, "file:///tmp/test.yaml", unknownDiag.Range, []Diagnostic{*unknownDiag})

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
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	// Open a file with a yaml-language-server modeline.
	// The server should NOT store it and NOT send diagnostics.
	foreignText := "# yaml-language-server: $schema=https://json.schemastore.org/github-workflow\nname: CI\non: push\n"
	client.sendNotification("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///tmp/workflow.yaml",
			LanguageID: "yaml",
			Version:    1,
			Text:       foreignText,
		},
	})

	// No diagnostics notification should be sent.
	// Verify by requesting completion — should return empty since doc is not in store.
	list := client.complete(2, "file:///tmp/workflow.yaml", 2, 0)
	if len(list.Items) != 0 {
		t.Errorf("expected no completions for foreign modeline file, got: %v", labelList(list.Items))
	}
}

// TestEditorSession_TemplateVarsSecretsSteps simulates an editor session
// where the user opens a scenario with vars, secrets, and steps, then
// requests template dot-completion for each.
func TestEditorSession_TemplateVarsSecretsSteps(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	docText := "schemaVersion: scenario/v1\ntitle: test\nvars:\n  myVar: hello\n  anotherVar: world\nsecrets:\n  apiKey: xxx\nsteps:\n  - id: login\n    title: login step\n    protocol: http\n  - id: fetchData\n    title: fetch data\n    request:\n      url: '{{vars.}}'\n"
	client.openDocument("file:///tmp/test.yaml", docText)

	// {{vars. -> myVar, anotherVar
	list := client.complete(10, "file:///tmp/test.yaml", 14, 19)
	labels := labelSet(list.Items)
	if !labels["myVar"] || !labels["anotherVar"] {
		t.Errorf("vars. completion: expected myVar and anotherVar, got: %v", labelList(list.Items))
	}

	// Edit to {{steps.
	newText := "schemaVersion: scenario/v1\ntitle: test\nvars:\n  myVar: hello\n  anotherVar: world\nsecrets:\n  apiKey: xxx\nsteps:\n  - id: login\n    title: login step\n    protocol: http\n  - id: fetchData\n    title: fetch data\n    request:\n      url: '{{steps.}}'\n"
	client.changeDocument("file:///tmp/test.yaml", 2, newText)

	list = client.complete(11, "file:///tmp/test.yaml", 14, 20)
	labels = labelSet(list.Items)
	if !labels["login"] || !labels["fetchData"] {
		t.Errorf("steps. completion: expected login and fetchData, got: %v", labelList(list.Items))
	}

	// Edit to {{secrets.
	newText2 := "schemaVersion: scenario/v1\ntitle: test\nvars:\n  myVar: hello\n  anotherVar: world\nsecrets:\n  apiKey: xxx\nsteps:\n  - id: login\n    title: login step\n    protocol: http\n  - id: fetchData\n    title: fetch data\n    request:\n      url: '{{secrets.}}'\n"
	client.changeDocument("file:///tmp/test.yaml", 3, newText2)

	list = client.complete(12, "file:///tmp/test.yaml", 14, 22)
	labels = labelSet(list.Items)
	if !labels["apiKey"] {
		t.Errorf("secrets. completion: expected apiKey, got: %v", labelList(list.Items))
	}
}

// TestEditorSession_TemplateVarsPartialFilter verifies partial-match filtering
// in template dot-completion (e.g., {{vars.my -> myVar only).
func TestEditorSession_TemplateVarsPartialFilter(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	docText := "schemaVersion: scenario/v1\ntitle: test\nvars:\n  myVar: hello\n  anotherVar: world\nsteps:\n  - title: step1\n    request:\n      url: '{{vars.my}}'\n"
	client.openDocument("file:///tmp/test.yaml", docText)

	list := client.complete(2, "file:///tmp/test.yaml", 8, 21)
	labels := labelSet(list.Items)
	if !labels["myVar"] {
		t.Errorf("expected myVar, got: %v", labelList(list.Items))
	}
	if labels["anotherVar"] {
		t.Errorf("should not include anotherVar when filtering by 'my'")
	}
}

// TestEditorSession_ConfigVarsCompletion verifies that vars from scenarigo.yaml
// are included in template completion candidates.
func TestEditorSession_ConfigVarsCompletion(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a config file with vars.
	configContent := "schemaVersion: config/v1\nvars:\n  configVar: from-config\n  anotherConfigVar: also\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "scenarigo.yaml"), []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	rootURI := fmt.Sprintf("file://%s", tmpDir)
	client.initialize(1, rootURI)

	// Scenario file with no local vars, but using {{vars.
	docText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    request:\n      url: '{{vars.}}'\n"
	uri := "file://" + filepath.Join(tmpDir, "test.yaml")
	client.openDocument(uri, docText)

	list := client.complete(2, uri, 5, 19)
	labels := labelSet(list.Items)
	if !labels["configVar"] || !labels["anotherConfigVar"] {
		t.Errorf("expected config vars, got: %v", labelList(list.Items))
	}

	// Check detail indicates config origin.
	for _, item := range list.Items {
		if item.Label == "configVar" && item.Detail != "(from scenarigo.yaml)" {
			t.Errorf("expected detail '(from scenarigo.yaml)' for configVar, got: %q", item.Detail)
		}
	}
}

// TestEditorSession_FullWorkflow simulates a realistic editor workflow:
// open file -> get diagnostics -> complete -> hover -> symbols -> references -> code action -> close.
func TestEditorSession_FullWorkflow(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	initResult := client.initialize(1, "file:///tmp")
	// Verify all expected capabilities are advertised.
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
	diags := client.openDocumentAndGetDiagnostics("file:///tmp/test.yaml", docText)
	if len(diags.Diagnostics) != 0 {
		t.Errorf("expected no diagnostics, got: %v", diagMessages(diags.Diagnostics))
	}

	// Hover on "protocol".
	hoverResp := client.hover(2, "file:///tmp/test.yaml", 7, 6)
	var hoverResult Hover
	if err := json.Unmarshal(hoverResp, &hoverResult); err != nil {
		t.Fatalf("unmarshal hover: %v", err)
	}
	if hoverResult.Contents.Value == "" {
		t.Error("expected non-empty hover content for 'protocol'")
	}

	// Document symbols.
	symResp := client.documentSymbol(3, "file:///tmp/test.yaml")
	var symbols []DocumentSymbol
	if err := json.Unmarshal(symResp, &symbols); err != nil {
		t.Fatalf("unmarshal symbols: %v", err)
	}
	names := collectSymbolNames(symbols)
	if !names["title"] || !names["vars"] || !names["steps"] {
		t.Errorf("expected title, vars, steps in symbols, got: %v", symbolNameList(symbols))
	}

	// References on "token" (line 3, char 4 = the key "token" under vars).
	refsResp := client.references(4, "file:///tmp/test.yaml", 3, 4)
	var locs []Location
	if err := json.Unmarshal(refsResp, &locs); err != nil {
		t.Fatalf("unmarshal locations: %v", err)
	}
	if len(locs) < 2 {
		t.Errorf("expected at least 2 references (decl + usage), got %d", len(locs))
	}

	// Close document.
	client.closeDocument("file:///tmp/test.yaml")

	// Shutdown.
	client.shutdown(5)
}
