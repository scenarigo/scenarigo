package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// TestEditorSession_EncodedWorkspaceURI checks that features reading files from
// disk work when the workspace path needs percent-encoding in URIs.
func TestEditorSession_EncodedWorkspaceURI(t *testing.T) {
	client := newRunningTestClient(t)
	dir := filepath.Join(t.TempDir(), "my project 日本")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "schemaVersion: config/v1\nvars:\n  configVar: from-config\n"
	if err := os.WriteFile(filepath.Join(dir, "scenarigo.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "included.yaml"), []byte("title: included\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := pathToURI(dir)
	if !strings.Contains(root, "%20") {
		t.Fatalf("root URI is not percent-encoded: %s", root)
	}
	client.initialize(1, root)

	uri := root + "/test.yaml"
	docText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - include: included.yaml\n    vars:\n      x: '{{vars.'\n"
	client.openDocument(uri, docText)

	list := client.complete(2, uri, 5, 17)
	if labels := labelList(list.Items); !slices.Contains(labels, "configVar") {
		t.Errorf("expected configVar from scenarigo.yaml in completions, got %v", labels)
	}

	var loc Location
	if err := json.Unmarshal(client.definition(3, uri, 3, 15), &loc); err != nil {
		t.Fatalf("decode definition: %v", err)
	}
	if want := root + "/included.yaml"; loc.URI != want {
		t.Errorf("definition URI = %q, want %q", loc.URI, want)
	}

	// An open scenarigo.yaml buffer must win over the file on disk even when
	// the client encodes its URI differently (Neovim uses lowercase hex).
	lowerHex := regexp.MustCompile(`%[0-9A-F]{2}`)
	configURI := lowerHex.ReplaceAllStringFunc(root+"/scenarigo.yaml", strings.ToLower)
	if configURI == root+"/scenarigo.yaml" {
		t.Fatalf("expected an upper-case escape in %s", root)
	}
	client.openDocument(configURI, "schemaVersion: config/v1\nvars:\n  bufferOnlyVar: 1\n")
	list = client.complete(4, uri, 5, 17)
	if labels := labelList(list.Items); !slices.Contains(labels, "bufferOnlyVar") {
		t.Errorf("expected bufferOnlyVar from the open buffer in completions, got %v", labels)
	}
}

// TestEditorSession_TemplateTriggerCharacters checks completion requests
// triggered by "{" and ".": inside a template expression they offer
// candidates, and elsewhere they offer nothing, because a client replaces the
// text after the trigger character only and would garble a file path or a
// value accepted from the list.
func TestEditorSession_TemplateTriggerCharacters(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)
	if err := os.WriteFile(filepath.Join(uriToPath(root), "steps.yaml"), []byte("title: included\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client.initialize(1, root)

	tests := []struct {
		name    string
		doc     string // $0 marks the cursor, right after the trigger character
		trigger string
		want    string // a label that must be offered; empty means no candidates
	}{
		{name: "first brace", doc: "schemaVersion: scenario/v1\nsteps:\n  - request:\n      url: '{$0'\n", trigger: "{"},
		{name: "template start", doc: "schemaVersion: scenario/v1\nsteps:\n  - request:\n      url: '{{$0'\n", trigger: "{", want: "vars"},
		{name: "template dot", doc: "schemaVersion: scenario/v1\nvars:\n  token: abc\nsteps:\n  - request:\n      url: '{{vars.$0'\n", trigger: ".", want: "token"},
		{name: "url", doc: "schemaVersion: scenario/v1\nsteps:\n  - request:\n      url: http://example.$0\n", trigger: "."},
		{name: "include file name", doc: "schemaVersion: scenario/v1\nsteps:\n  - include: steps.$0\n", trigger: "."},
		{name: "include relative path", doc: "schemaVersion: scenario/v1\nsteps:\n  - include: .$0\n", trigger: "."},
		{name: "scenarios item", doc: "schemaVersion: config/v1\nscenarios:\n  - steps.$0\n", trigger: "."},
		{name: "bool value", doc: "schemaVersion: config/v1\noutput:\n  verbose: .$0\n", trigger: "."},
		{name: "flow mapping", doc: "schemaVersion: scenario/v1\nsteps:\n  - request: {$0\n", trigger: "{"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, line, char, ok := parseCursorMarker(tt.doc)
			if !ok {
				t.Fatal("no cursor marker")
			}
			uri := file(fmt.Sprintf("test%d.yaml", i))
			client.openDocument(uri, text)
			list := client.completeWith(i+2, CompletionParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Position:     Position{Line: line, Character: char},
				Context:      &CompletionContext{TriggerKind: CompletionTriggerKindTriggerCharacter, TriggerCharacter: tt.trigger},
			})
			labels := labelList(list.Items)
			if tt.want == "" {
				if len(labels) != 0 {
					t.Errorf("expected no completions, got %v", labels)
				}
				return
			}
			if !slices.Contains(labels, tt.want) {
				t.Errorf("expected %q in completions, got %v", tt.want, labels)
			}
		})
	}

	// Invoking completion explicitly still offers file names.
	uri := file("invoked.yaml")
	text, line, char, _ := parseCursorMarker("schemaVersion: scenario/v1\nsteps:\n  - include: st$0\n")
	client.openDocument(uri, text)
	list := client.completeWith(100, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: char},
		Context:      &CompletionContext{TriggerKind: CompletionTriggerKindInvoked},
	})
	if labels := labelList(list.Items); !slices.Contains(labels, "steps.yaml") {
		t.Errorf("expected steps.yaml for an invoked completion, got %v", labels)
	}
}

// TestEditorSession_ExamplesHaveNoErrorsOrWarnings opens every YAML file under
// examples/ with its example directory as the workspace root, so that
// scenarigo.yaml is found as in a real session, and requires that none of
// these working scenarios gets an error or a warning. scenarigo reads a file
// without schemaVersion as scenario/v1 while the server stays silent on it,
// so such a scenario is checked with the version added; ytt data files
// (*.ytt.yaml without schemaVersion) are not scenarios and are skipped.
func TestEditorSession_ExamplesHaveNoErrorsOrWarnings(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")
	configs, err := filepath.Glob(filepath.Join(examplesDir, "*", "scenarigo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) == 0 {
		t.Fatal("no examples found")
	}
	opened := 0
	for _, config := range configs {
		dir, err := filepath.Abs(filepath.Dir(config))
		if err != nil {
			t.Fatal(err)
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			client := newRunningTestClient(t)
			client.initialize(1, pathToURI(dir))
			err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || filepath.Ext(path) != ".yaml" {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				rel, _ := filepath.Rel(dir, path)
				text, added := string(data), 0
				if !strings.Contains(text, "schemaVersion:") {
					if strings.HasSuffix(path, ".ytt.yaml") {
						t.Logf("%s: ytt data file, skipped", rel)
						return nil
					}
					text, added = "schemaVersion: scenario/v1\n"+text, 1
					t.Logf("%s: checked with schemaVersion: scenario/v1 added", rel)
				}
				diags := client.openDocumentAndGetDiagnostics(pathToURI(path), text)
				opened++
				for _, d := range diags.Diagnostics {
					if d.Severity == DiagnosticSeverityError || d.Severity == DiagnosticSeverityWarning {
						t.Errorf("%s:%d:%d: severity %d: %s", rel, d.Range.Start.Line+1-added, d.Range.Start.Character+1, d.Severity, d.Message)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	if opened == 0 {
		t.Fatal("no example files were opened")
	}
}

// TestEditorSession_ConfigChangeRepublishes checks that editing scenarigo.yaml
// updates the diagnostics of the open scenarios that use its names.
func TestEditorSession_ConfigChangeRepublishes(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)
	client.initialize(1, root)

	scenario := file("test.yaml")
	diags := client.openDocumentAndGetDiagnostics(scenario, "schemaVersion: scenario/v1\nsteps:\n  - protocol: http\n    request:\n      method: GET\n      url: '{{vars.host}}'\n")
	if len(diags.Diagnostics) != 1 {
		t.Fatalf("expected the undefined vars.host, got %v", diagMessages(diags.Diagnostics))
	}

	config := file("scenarigo.yaml")
	client.sendNotification("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: config, LanguageID: "yaml", Version: 1, Text: "schemaVersion: config/v1\nvars:\n  host: localhost\n"},
	})
	// Collect the notifications that arrive before the response to a request.
	client.sendRequest(2, "textDocument/hover", HoverParams{TextDocument: TextDocumentIdentifier{URI: config}})
	got := map[string]PublishDiagnosticsParams{}
	for {
		raw := client.readMessage()
		if !isNotification(raw) {
			break
		}
		var n struct {
			Params PublishDiagnosticsParams `json:"params"`
		}
		if err := json.Unmarshal(raw, &n); err != nil {
			t.Fatal(err)
		}
		got[n.Params.URI] = n.Params
	}
	if p, ok := got[scenario]; !ok {
		t.Fatalf("diagnostics of %s were not published again; got %v", scenario, got)
	} else if len(p.Diagnostics) != 0 {
		t.Errorf("expected no diagnostics once scenarigo.yaml defines vars.host, got %v", diagMessages(p.Diagnostics))
	}
}

// TestEditorSession_IncludeIndex checks that the scenarios including a file
// come from one walk of the workspace, updated from the open buffers.
func TestEditorSession_IncludeIndex(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)
	client.initialize(1, root)

	child := file("child.yaml")
	childText := "schemaVersion: scenario/v1\nsteps:\n  - protocol: grpc\n    request:\n      client: '{{plugins.p.Client}}'\n"
	diags := client.openDocumentAndGetDiagnostics(child, childText)
	if len(diags.Diagnostics) != 1 {
		t.Fatalf("expected the undeclared plugin, got %v", diagMessages(diags.Diagnostics))
	}

	// A file written on disk after the walk is not seen until it is opened.
	parentText := "schemaVersion: scenario/v1\nplugins:\n  p: p.so\nsteps:\n  - include: child.yaml\n"
	if err := os.WriteFile(uriToPath(file("parent.yaml")), []byte(parentText), 0o600); err != nil {
		t.Fatal(err)
	}
	change := func(version int) PublishDiagnosticsParams {
		t.Helper()
		client.sendNotification("textDocument/didChange", DidChangeTextDocumentParams{
			TextDocument:   VersionedTextDocumentIdentifier{TextDocumentIdentifier: TextDocumentIdentifier{URI: child}, Version: version},
			ContentChanges: []TextDocumentContentChangeEvent{{Text: childText}},
		})
		var n struct {
			Params PublishDiagnosticsParams `json:"params"`
		}
		if err := json.Unmarshal(client.readMessage(), &n); err != nil {
			t.Fatal(err)
		}
		if n.Params.URI != child {
			t.Fatalf("expected diagnostics for %s, got %s", child, n.Params.URI)
		}
		return n.Params
	}
	if p := change(2); len(p.Diagnostics) != 1 {
		t.Errorf("the workspace was walked again on a change: got %v", diagMessages(p.Diagnostics))
	}
	client.openDocument(file("parent.yaml"), parentText)
	if p := change(3); len(p.Diagnostics) != 0 {
		t.Errorf("expected no diagnostics once the including buffer is open, got %v", diagMessages(p.Diagnostics))
	}

	// Unsaved edits of a closed buffer are dropped: the file on disk counts again.
	client.changeDocument(file("parent.yaml"), 2, "schemaVersion: scenario/v1\nsteps: []\n")
	if p := change(4); len(p.Diagnostics) != 1 {
		t.Errorf("expected the undeclared plugin while the buffer drops the include, got %v", diagMessages(p.Diagnostics))
	}
	client.closeDocument(file("parent.yaml"))
	if p := change(5); len(p.Diagnostics) != 0 {
		t.Errorf("expected the file on disk to count after the buffer is closed, got %v", diagMessages(p.Diagnostics))
	}
}

// TestEditorSession_Rename applies the edits of a rename and checks the
// resulting text, and that the capability follows the client's prepareSupport.
func TestEditorSession_Rename(t *testing.T) {
	t.Run("capability", func(t *testing.T) {
		for _, prepare := range []bool{false, true} {
			client := newRunningTestClient(t)
			root, _ := newWorkspace(t)
			result := client.initializeWith(1, InitializeParams{
				RootURI: root,
				Capabilities: ClientCapabilities{TextDocument: &TextDocumentClientCapabilities{
					Rename: &RenameClientCapabilities{PrepareSupport: prepare},
				}},
			})
			got, _ := json.Marshal(result.Capabilities.RenameProvider)
			want := "true"
			if prepare {
				want = `{"prepareProvider":true}`
			}
			if string(got) != want {
				t.Errorf("prepareSupport %v: renameProvider = %s, want %s", prepare, got, want)
			}
		}
	})

	t.Run("edits", func(t *testing.T) {
		client := newRunningTestClient(t)
		root, file := newWorkspace(t)
		client.initialize(1, root)
		uri := file("test.yaml")
		text := "schemaVersion: scenario/v1\n" +
			"vars:\n" +
			"  user: 日本\n" +
			"steps:\n" +
			"  - id: login\n" +
			"    protocol: http\n" +
			"    request:\n" +
			"      method: GET\n" +
			"      url: '日本/{{vars.user}}/{{size(vars.user) ?? vars.username}}'\n" +
			"    bind:\n" +
			"      vars:\n" +
			"        user: '{{response.body.user}}'\n"
		want := strings.ReplaceAll(strings.ReplaceAll(text, "vars.user}", "vars.account}"), "vars.user)", "vars.account)")
		want = strings.Replace(want, "  user: 日本", "  account: 日本", 1)
		want = strings.Replace(want, "        user: '{{response", "        account: '{{response", 1)
		client.openDocument(uri, text)

		resp, respErr := client.rename(2, uri, 8, len(utf16.Encode([]rune("      url: '日本/{{vars.u"))), "account")
		if respErr != nil {
			t.Fatalf("rename: %s", respErr.Message)
		}
		var edit WorkspaceEdit
		if err := json.Unmarshal(resp, &edit); err != nil {
			t.Fatal(err)
		}
		if got := applyEdits(t, text, edit.Changes[uri]); got != want {
			t.Errorf("after rename:\n%s\nwant:\n%s", got, want)
		}
	})
}

// applyEdits applies non-overlapping single-line edits whose characters
// count UTF-16 code units.
func applyEdits(t *testing.T, text string, edits []TextEdit) string {
	t.Helper()
	lines := strings.Split(text, "\n")
	slices.SortFunc(edits, func(a, b TextEdit) int { return -compareRanges(a.Range, b.Range) })
	for _, e := range edits {
		if e.Range.Start.Line != e.Range.End.Line {
			t.Fatalf("multi-line edit %+v", e.Range)
		}
		units := utf16.Encode([]rune(lines[e.Range.Start.Line]))
		before := string(utf16.Decode(units[:e.Range.Start.Character]))
		after := string(utf16.Decode(units[e.Range.End.Character:]))
		lines[e.Range.Start.Line] = before + e.NewText + after
	}
	return strings.Join(lines, "\n")
}
