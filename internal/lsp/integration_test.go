package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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

	t.Run("no workspace root", func(t *testing.T) {
		client := newRunningTestClient(t)
		client.initialize(1, "")
		uri := pathToURI(filepath.Join(t.TempDir(), "test.yaml"))
		client.openDocument(uri, "schemaVersion: scenario/v1\nvars:\n  user: alice\n")
		_, respErr := client.rename(2, uri, 2, 3, "account")
		if respErr == nil || !strings.Contains(respErr.Message, "workspace root") {
			t.Errorf("expected a refusal without a workspace root, got %v", respErr)
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

// sync sends a request and returns what the server sent before answering
// it: the last diagnostics published for each URI and the requests the
// server made.
func (c *testClient) sync(id int) (map[string]PublishDiagnosticsParams, []Request) {
	c.t.Helper()
	c.sendRequest(id, "shutdownCheck/ping", struct{}{})
	diags := map[string]PublishDiagnosticsParams{}
	var requests []Request
	for {
		raw := c.readMessage()
		var msg Request
		if err := json.Unmarshal(raw, &msg); err != nil {
			c.t.Fatalf("unmarshal message: %v", err)
		}
		switch {
		case msg.Method == "":
			if msg.ID == nil || string(*msg.ID) != strconv.Itoa(id) {
				c.t.Fatalf("unexpected response %s while waiting for %d", raw, id)
			}
			return diags, requests // the answer to the ping
		case msg.ID != nil:
			requests = append(requests, msg)
		case msg.Method == "textDocument/publishDiagnostics":
			var p PublishDiagnosticsParams
			if err := json.Unmarshal(msg.Params, &p); err != nil {
				c.t.Fatalf("unmarshal diagnostics: %v", err)
			}
			diags[p.URI] = p
		}
	}
}

func (c *testClient) watchedFilesChanged(changes ...FileEvent) {
	c.t.Helper()
	c.sendNotification("workspace/didChangeWatchedFiles", DidChangeWatchedFilesParams{Changes: changes})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestEditorSession_WatchedFiles checks that changes on disk reported by
// workspace/didChangeWatchedFiles update the open documents.
func TestEditorSession_WatchedFiles(t *testing.T) {
	t.Run("scenarigo.yaml", func(t *testing.T) {
		client := newRunningTestClient(t)
		root, file := newWorkspace(t)
		client.initialize(1, root)
		scenario := file("test.yaml")
		diags := client.openDocumentAndGetDiagnostics(scenario, "schemaVersion: scenario/v1\nsteps:\n  - protocol: http\n    request:\n      method: GET\n      url: '{{vars.host}}'\n")
		if len(diags.Diagnostics) != 1 {
			t.Fatalf("expected the undefined vars.host, got %v", diagMessages(diags.Diagnostics))
		}

		writeFile(t, uriToPath(file("scenarigo.yaml")), "schemaVersion: config/v1\nvars:\n  host: localhost\n")
		client.watchedFilesChanged(FileEvent{URI: file("scenarigo.yaml"), Type: FileChangeTypeCreated})
		got, _ := client.sync(2)
		if p, ok := got[scenario]; !ok || len(p.Diagnostics) != 0 {
			t.Errorf("expected %s to be published again without diagnostics, got %v", scenario, got)
		}
	})

	t.Run("plugin source", func(t *testing.T) {
		client := newRunningTestClient(t)
		root, file := newWorkspace(t)
		dir := uriToPath(root)
		writeFile(t, filepath.Join(dir, "scenarigo.yaml"), "schemaVersion: config/v1\nplugins:\n  p.so:\n    src: ./plugin\n")
		src := filepath.Join(dir, "plugin", "main.go")
		writeFile(t, src, "package main\n\nfunc Old() {}\n")
		client.initialize(1, root)
		uri := file("test.yaml")
		text, line, char, _ := parseCursorMarker("schemaVersion: scenario/v1\nplugins:\n  p: p.so\nsteps:\n  - request:\n      client: '{{plugins.p.$0'\n")
		client.openDocument(uri, text)
		if labels := labelList(client.complete(2, uri, line, char).Items); !slices.Contains(labels, "Old") {
			t.Fatalf("expected Old, got %v", labels)
		}

		writeFile(t, src, "package main\n\nfunc New() {}\n")
		client.watchedFilesChanged(FileEvent{URI: pathToURI(src), Type: FileChangeTypeChanged})
		client.sync(3)
		labels := labelList(client.complete(4, uri, line, char).Items)
		if !slices.Contains(labels, "New") || slices.Contains(labels, "Old") {
			t.Errorf("expected the symbols of the changed source, got %v", labels)
		}
	})

	t.Run("including scenario", func(t *testing.T) {
		client := newRunningTestClient(t)
		root, file := newWorkspace(t)
		client.initialize(1, root)
		child := file("child.yaml")
		diags := client.openDocumentAndGetDiagnostics(child, "schemaVersion: scenario/v1\nsteps:\n  - protocol: grpc\n    request:\n      client: '{{plugins.p.Client}}'\n")
		if len(diags.Diagnostics) != 1 {
			t.Fatalf("expected the undeclared plugin, got %v", diagMessages(diags.Diagnostics))
		}

		parent := file("parent.yaml")
		writeFile(t, uriToPath(parent), "schemaVersion: scenario/v1\nplugins:\n  p: p.so\nsteps:\n  - include: child.yaml\n")
		client.watchedFilesChanged(FileEvent{URI: parent, Type: FileChangeTypeCreated})
		got, _ := client.sync(2)
		if p, ok := got[child]; !ok || len(p.Diagnostics) != 0 {
			t.Errorf("expected %s to be published again without diagnostics, got %v", child, got)
		}

		if err := os.Remove(uriToPath(parent)); err != nil {
			t.Fatal(err)
		}
		client.watchedFilesChanged(FileEvent{URI: parent, Type: FileChangeTypeDeleted})
		got, _ = client.sync(3)
		if p, ok := got[child]; !ok || len(p.Diagnostics) != 1 {
			t.Errorf("expected the undeclared plugin again after the deletion, got %v", got)
		}
	})
}

// TestEditorSession_RegisterWatchers checks the dynamic registration of the
// file watchers, and that the client's answer gets no reply.
func TestEditorSession_RegisterWatchers(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		client := newRunningTestClient(t)
		root, _ := newWorkspace(t)
		client.initializeWith(1, InitializeParams{
			RootURI: root,
			Capabilities: ClientCapabilities{Workspace: &WorkspaceClientCapabilities{
				DidChangeWatchedFiles: &DidChangeWatchedFilesClientCapabilities{DynamicRegistration: dynamic},
			}},
		})
		client.sendNotification("initialized", struct{}{})
		_, requests := client.sync(2)
		if !dynamic {
			if len(requests) != 0 {
				t.Errorf("without dynamic registration: unexpected requests %v", requests)
			}
			continue
		}
		if len(requests) != 1 || requests[0].Method != "client/registerCapability" {
			t.Fatalf("expected one client/registerCapability request, got %v", requests)
		}
		var params RegistrationParams
		if err := json.Unmarshal(requests[0].Params, &params); err != nil {
			t.Fatal(err)
		}
		if len(params.Registrations) != 1 || params.Registrations[0].Method != "workspace/didChangeWatchedFiles" {
			t.Fatalf("unexpected registrations %+v", params.Registrations)
		}
		opts, err := json.Marshal(params.Registrations[0].RegisterOptions)
		if err != nil {
			t.Fatal(err)
		}
		for _, pattern := range []string{"**/*.{yaml,yml}", "**/*.go", "**/*.proto", `"**/*","kind":5`} {
			if !strings.Contains(string(opts), pattern) {
				t.Errorf("watchers %s do not include %s", opts, pattern)
			}
		}

		// The client answers; the server must not answer the answer.
		client.sendRaw(`{"jsonrpc":"2.0","id":` + string(*requests[0].ID) + `,"result":null}`)
		if _, requests := client.sync(3); len(requests) != 0 {
			t.Errorf("unexpected requests after the answer: %v", requests)
		}

		// A folder change replaces the registration, so that the client
		// watches the new folders and stops watching removed ones.
		var first RegistrationParams
		_ = json.Unmarshal(requests[0].Params, &first)
		client.sendNotification("workspace/didChangeWorkspaceFolders", DidChangeWorkspaceFoldersParams{
			Event: WorkspaceFoldersChangeEvent{Added: []WorkspaceFolder{{URI: pathToURI(t.TempDir()), Name: "new"}}},
		})
		_, requests = client.sync(4)
		if len(requests) != 2 || requests[0].Method != "client/unregisterCapability" || requests[1].Method != "client/registerCapability" {
			t.Fatalf("expected the registration replaced for the new folder, got %v", requests)
		}
		var unreg UnregistrationParams
		if err := json.Unmarshal(requests[0].Params, &unreg); err != nil {
			t.Fatal(err)
		}
		if len(unreg.Unregisterations) != 1 || unreg.Unregisterations[0].ID != first.Registrations[0].ID {
			t.Errorf("expected %s unregistered, got %+v", first.Registrations[0].ID, unreg)
		}
	}
}

// TestEditorSession_WorkspaceFolders checks that each document uses the
// scenarigo.yaml of the workspace folder that contains it.
func TestEditorSession_WorkspaceFolders(t *testing.T) {
	base := t.TempDir()
	folder := func(name, varName string) string {
		dir := filepath.Join(base, name)
		writeFile(t, filepath.Join(dir, "scenarigo.yaml"), "schemaVersion: config/v1\nvars:\n  "+varName+": 1\n")
		return pathToURI(dir)
	}
	a, b, c := folder("a", "fromA"), folder("b", "fromB"), folder("c", "fromC")

	client := newRunningTestClient(t)
	client.initializeWith(1, InitializeParams{
		RootURI:          a,
		WorkspaceFolders: []WorkspaceFolder{{URI: a, Name: "a"}, {URI: b, Name: "b"}},
	})
	text, line, char, _ := parseCursorMarker("schemaVersion: scenario/v1\nsteps:\n  - request:\n      url: '{{vars.$0'\n")
	complete := func(id int, root string) []string {
		t.Helper()
		uri := root + "/test.yaml"
		client.openDocument(uri, text)
		return labelList(client.complete(id, uri, line, char).Items)
	}
	if got := complete(2, a); !slices.Contains(got, "fromA") || slices.Contains(got, "fromB") {
		t.Errorf("folder a: got %v", got)
	}
	if got := complete(3, b); !slices.Contains(got, "fromB") || slices.Contains(got, "fromA") {
		t.Errorf("folder b: got %v", got)
	}

	client.sendNotification("workspace/didChangeWorkspaceFolders", DidChangeWorkspaceFoldersParams{
		Event: WorkspaceFoldersChangeEvent{Added: []WorkspaceFolder{{URI: c, Name: "c"}}, Removed: []WorkspaceFolder{{URI: b, Name: "b"}}},
	})
	client.sync(4)
	if got := complete(5, c); !slices.Contains(got, "fromC") {
		t.Errorf("added folder c: got %v", got)
	}
	if got := complete(6, b); slices.Contains(got, "fromB") {
		t.Errorf("removed folder b still used: got %v", got)
	}
}

// TestEditorSession_FolderSelection checks which scenarigo.yaml documents use
// when workspace folders nest, overlap the documents or are removed.
func TestEditorSession_FolderSelection(t *testing.T) {
	scenario := "schemaVersion: scenario/v1\nsteps:\n  - protocol: grpc\n    request:\n      client: '{{plugins.p.Client}}'\n      target: '{{vars.host}}'\n"
	infoOn := func(diags PublishDiagnosticsParams, name string) bool {
		for _, d := range diags.Diagnostics {
			if strings.Contains(d.Message, name) {
				return true
			}
		}
		return false
	}

	t.Run("nested folders", func(t *testing.T) {
		ws := t.TempDir()
		writeFile(t, filepath.Join(ws, "scenarigo.yaml"), "schemaVersion: config/v1\nvars:\n  host: outer\n")
		writeFile(t, filepath.Join(ws, "main.yaml"), "schemaVersion: scenario/v1\nplugins:\n  p: p.so\nsteps:\n  - include: sub/child.yaml\n")
		client := newRunningTestClient(t)
		client.initializeWith(1, InitializeParams{WorkspaceFolders: []WorkspaceFolder{
			{URI: pathToURI(ws), Name: "ws"}, {URI: pathToURI(filepath.Join(ws, "sub")) + "/", Name: "sub"},
		}})
		diags := client.openDocumentAndGetDiagnostics(pathToURI(filepath.Join(ws, "sub", "child.yaml")), scenario)
		if len(diags.Diagnostics) != 0 {
			t.Errorf("the inner folder has no scenarigo.yaml and the outer folder includes the file: got %v", diagMessages(diags.Diagnostics))
		}

		// An inner scenarigo.yaml wins.
		writeFile(t, filepath.Join(ws, "sub", "scenarigo.yaml"), "schemaVersion: config/v1\nvars:\n  other: 1\n")
		diags = client.openDocumentAndGetDiagnostics(pathToURI(filepath.Join(ws, "sub", "other.yaml")), "schemaVersion: scenario/v1\nvars:\n  a: '{{vars.other}}/{{vars.host}}'\n")
		if infoOn(diags, "vars.other") || !infoOn(diags, "vars.host") {
			t.Errorf("expected the inner scenarigo.yaml, got %v", diagMessages(diags.Diagnostics))
		}
	})

	t.Run("each document is published with its folder", func(t *testing.T) {
		base := t.TempDir()
		a, b := filepath.Join(base, "a"), filepath.Join(base, "b")
		writeFile(t, filepath.Join(a, "scenarigo.yaml"), "schemaVersion: config/v1\nvars:\n  fromA: 1\n")
		writeFile(t, filepath.Join(b, "scenarigo.yaml"), "schemaVersion: config/v1\nvars:\n  fromB: 1\n")
		client := newRunningTestClient(t)
		client.initializeWith(1, InitializeParams{WorkspaceFolders: []WorkspaceFolder{{URI: pathToURI(a), Name: "a"}, {URI: pathToURI(b), Name: "b"}}})
		docA, docB := pathToURI(filepath.Join(a, "s.yaml")), pathToURI(filepath.Join(b, "s.yaml"))
		client.openDocument(docA, "schemaVersion: scenario/v1\nvars:\n  x: '{{vars.fromA}}'\n")
		client.openDocument(docB, "schemaVersion: scenario/v1\nvars:\n  x: '{{vars.fromB}}'\n")

		// A watched-file event publishes every document with its own folder.
		client.watchedFilesChanged(FileEvent{URI: pathToURI(filepath.Join(a, "other.yaml")), Type: FileChangeTypeCreated})
		got, _ := client.sync(2)
		for _, uri := range []string{docA, docB} {
			if p, ok := got[uri]; !ok || len(p.Diagnostics) != 0 {
				t.Errorf("%s: expected no diagnostics, got %v", uri, got[uri])
			}
		}

		// A change of a's scenarigo.yaml publishes a's documents only.
		client.sendNotification("textDocument/didOpen", DidOpenTextDocumentParams{TextDocument: TextDocumentItem{
			URI: pathToURI(filepath.Join(a, "scenarigo.yaml")), LanguageID: "yaml", Version: 1, Text: "schemaVersion: config/v1\nvars:\n  renamed: 1\n",
		}})
		got, _ = client.sync(3)
		if p, ok := got[docA]; !ok || !infoOn(p, "vars.fromA") {
			t.Errorf("expected %s again with the new scenarigo.yaml, got %v", docA, got)
		}
		if _, ok := got[docB]; ok {
			t.Errorf("%s does not use a's scenarigo.yaml but was published", docB)
		}

		// After b is removed, its document uses no folder, not a's.
		client.changeDocument(docB, 2, "schemaVersion: scenario/v1\nvars:\n  x: '{{vars.renamed}}'\n")
		client.sendNotification("workspace/didChangeWorkspaceFolders", DidChangeWorkspaceFoldersParams{
			Event: WorkspaceFoldersChangeEvent{Removed: []WorkspaceFolder{{URI: pathToURI(b), Name: "b"}}},
		})
		got, _ = client.sync(4)
		if p := got[docB]; !infoOn(p, "vars.renamed") {
			t.Errorf("expected %s without a's scenarigo.yaml, got %v", docB, p)
		}
	})

	t.Run("a removed folder is indexed again when added back", func(t *testing.T) {
		ws := t.TempDir()
		writeFile(t, filepath.Join(ws, "child.yaml"), scenario)
		client := newRunningTestClient(t)
		folder := WorkspaceFolder{URI: pathToURI(ws), Name: "ws"}
		client.initializeWith(1, InitializeParams{WorkspaceFolders: []WorkspaceFolder{folder}})
		child := pathToURI(filepath.Join(ws, "child.yaml"))
		if diags := client.openDocumentAndGetDiagnostics(child, scenario); !infoOn(diags, `plugin "p"`) {
			t.Fatalf("expected the undeclared plugin, got %v", diagMessages(diags.Diagnostics))
		}
		client.sendNotification("workspace/didChangeWorkspaceFolders", DidChangeWorkspaceFoldersParams{Event: WorkspaceFoldersChangeEvent{Removed: []WorkspaceFolder{folder}}})
		client.sync(2)
		// Written while the folder is not part of the workspace: no event.
		writeFile(t, filepath.Join(ws, "main.yaml"), "schemaVersion: scenario/v1\nplugins:\n  p: p.so\nsteps:\n  - include: child.yaml\n")
		client.sendNotification("workspace/didChangeWorkspaceFolders", DidChangeWorkspaceFoldersParams{Event: WorkspaceFoldersChangeEvent{Added: []WorkspaceFolder{folder}}})
		got, _ := client.sync(3)
		if p := got[child]; infoOn(p, `plugin "p"`) {
			t.Errorf("expected the includer written while the folder was away, got %v", p)
		}
	})
}

// TestEditorSession_WatchedDirectories checks events for directories and
// plugin sources given with an absolute path.
func TestEditorSession_WatchedDirectories(t *testing.T) {
	t.Run("deleted directory", func(t *testing.T) {
		client := newRunningTestClient(t)
		root, file := newWorkspace(t)
		dir := uriToPath(root)
		writeFile(t, filepath.Join(dir, "flows", "parent.yaml"), "schemaVersion: scenario/v1\nplugins:\n  p: p.so\nsteps:\n  - include: ../child.yaml\n")
		client.initialize(1, root)
		child := file("child.yaml")
		if diags := client.openDocumentAndGetDiagnostics(child, "schemaVersion: scenario/v1\nsteps:\n  - protocol: grpc\n    request:\n      client: '{{plugins.p.Client}}'\n"); len(diags.Diagnostics) != 0 {
			t.Fatalf("expected no diagnostics, got %v", diagMessages(diags.Diagnostics))
		}
		if err := os.RemoveAll(filepath.Join(dir, "flows")); err != nil {
			t.Fatal(err)
		}
		client.watchedFilesChanged(FileEvent{URI: pathToURI(filepath.Join(dir, "flows")), Type: FileChangeTypeDeleted})
		got, _ := client.sync(2)
		if p := got[child]; len(p.Diagnostics) != 1 {
			t.Errorf("expected the undeclared plugin once flows/ is gone, got %v", p)
		}
		// The include index forgot flows/parent.yaml: renaming is not refused.
		client.changeDocument(child, 2, "schemaVersion: scenario/v1\nvars:\n  user: x\n")
		if _, respErr := client.rename(3, child, 2, 3, "account"); respErr != nil {
			t.Errorf("rename still refused after flows/ was deleted: %s", respErr.Message)
		}
	})

	t.Run("absolute plugin source", func(t *testing.T) {
		client := newRunningTestClient(t)
		root, file := newWorkspace(t)
		dir := uriToPath(root)
		src := filepath.Join(dir, "plugin")
		writeFile(t, filepath.Join(dir, "scenarigo.yaml"), "schemaVersion: config/v1\nplugins:\n  p.so:\n    src: "+src+"/\n")
		writeFile(t, filepath.Join(src, "main.go"), "package main\n\nfunc Old() {}\n")
		client.initialize(1, root)
		uri := file("test.yaml")
		text, line, char, _ := parseCursorMarker("schemaVersion: scenario/v1\nplugins:\n  p: p.so\nsteps:\n  - request:\n      client: '{{plugins.p.$0'\n")
		client.openDocument(uri, text)
		if labels := labelList(client.complete(2, uri, line, char).Items); !slices.Contains(labels, "Old") {
			t.Fatalf("expected Old, got %v", labels)
		}
		writeFile(t, filepath.Join(src, "main.go"), "package main\n\nfunc New() {}\n")
		client.watchedFilesChanged(FileEvent{URI: pathToURI(filepath.Join(src, "main.go")), Type: FileChangeTypeChanged})
		client.sync(3)
		if labels := labelList(client.complete(4, uri, line, char).Items); !slices.Contains(labels, "New") {
			t.Errorf("expected New, got %v", labels)
		}
	})
}

// TestEditorSession_EmptyMethod checks that a request with an empty method
// still gets an error, while the client's answers get none.
func TestEditorSession_EmptyMethod(t *testing.T) {
	client := newRunningTestClient(t)
	client.sendRaw(`{"jsonrpc":"2.0","id":7,"method":""}`)
	resp := client.readError()
	if resp.Error.Code != codeMethodNotFound || string(*resp.ID) != "7" {
		t.Errorf("unexpected answer %+v", resp)
	}
}

// TestEditorSession_WatchedPaths checks that the include index follows
// directories that appear and is not rebuilt for other files that come and go.
func TestEditorSession_WatchedPaths(t *testing.T) {
	childText := "schemaVersion: scenario/v1\nsteps:\n  - protocol: grpc\n    request:\n      client: '{{plugins.p.Client}}'\n"
	parentText := "schemaVersion: scenario/v1\nplugins:\n  p: p.so\nsteps:\n  - include: ../child.yaml\n"

	t.Run("a file that comes and goes keeps the index", func(t *testing.T) {
		client := newRunningTestClient(t)
		root, file := newWorkspace(t)
		dir := uriToPath(root)
		client.initialize(1, root)
		child := file("child.yaml")
		if diags := client.openDocumentAndGetDiagnostics(child, childText); len(diags.Diagnostics) != 1 {
			t.Fatalf("expected the undeclared plugin, got %v", diagMessages(diags.Diagnostics))
		}
		// Written without an event: only a new walk of the workspace sees it.
		writeFile(t, filepath.Join(dir, "flows", "parent.yaml"), parentText)
		probe := pathToURI(filepath.Join(dir, "4913")) // nvim's write test file
		client.watchedFilesChanged(FileEvent{URI: probe, Type: FileChangeTypeCreated}, FileEvent{URI: probe, Type: FileChangeTypeDeleted})
		got, _ := client.sync(2)
		if p := got[child]; len(p.Diagnostics) != 1 {
			t.Errorf("the workspace was walked again for a file that came and went: got %v", p)
		}
	})

	t.Run("a directory that appears is indexed", func(t *testing.T) {
		client := newRunningTestClient(t)
		root, file := newWorkspace(t)
		dir := uriToPath(root)
		client.initialize(1, root)
		child := file("child.yaml")
		if diags := client.openDocumentAndGetDiagnostics(child, childText); len(diags.Diagnostics) != 1 {
			t.Fatalf("expected the undeclared plugin, got %v", diagMessages(diags.Diagnostics))
		}
		writeFile(t, filepath.Join(dir, "flows", "parent.yaml"), parentText)
		client.watchedFilesChanged(FileEvent{URI: pathToURI(filepath.Join(dir, "flows")), Type: FileChangeTypeCreated})
		got, _ := client.sync(2)
		if p := got[child]; len(p.Diagnostics) != 0 {
			t.Errorf("expected the scenario of the new directory to include the file, got %v", p)
		}
	})
}

// TestEditorSession_GRPCProtoExample checks examples/grpc/scenarios/proto.yaml:
// the steps whose options list proto files complete services, methods and
// message fields, and the first step, which uses reflection, gets nothing
// from the proto files.
func TestEditorSession_GRPCProtoExample(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples", "grpc"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "scenarios", "proto.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	lines := strings.Split(text, "\n")
	// at returns the position after "key: " on the n-th (0-based) line with the key.
	at := func(key string, n int) (int, int) {
		t.Helper()
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), key+":") {
				if n == 0 {
					return i, strings.Index(l, key+":") + len(key) + 2
				}
				n--
			}
		}
		t.Fatalf("no %s line", key)
		return 0, 0
	}

	client := newRunningTestClient(t)
	client.initialize(1, pathToURI(dir))
	uri := pathToURI(path)
	diags := client.openDocumentAndGetDiagnostics(uri, text)
	for _, d := range diags.Diagnostics {
		if d.Severity == DiagnosticSeverityError || d.Severity == DiagnosticSeverityWarning {
			t.Errorf("line %d: %s", d.Range.Start.Line+1, d.Message)
		}
	}

	const ping = "scenarigo.examples.grpc.service.Ping"
	line, char := at("service", 0)
	if labels := labelList(client.complete(2, uri, line, char).Items); slices.Contains(labels, ping) {
		t.Errorf("step 1 uses reflection, but got services %v", labels)
	}
	line, char = at("service", 1)
	if labels := labelList(client.complete(3, uri, line, char).Items); !slices.Contains(labels, ping) {
		t.Errorf("step 2: expected %s, got %v", ping, labels)
	}
	line, char = at("method", 1)
	if labels := labelList(client.complete(4, uri, line, char).Items); !slices.Contains(labels, "Ping") {
		t.Errorf("step 2: expected the Ping method, got %v", labels)
	}
	line, _ = at("messageId", 0)
	char = strings.Index(lines[line], "messageId")
	if labels := labelList(client.complete(5, uri, line, char).Items); !slices.Contains(labels, "messageId") || !slices.Contains(labels, "messageBody") {
		t.Errorf("step 3: expected the EchoRequest fields, got %v", labels)
	}
}

// TestEditorSession_GRPCProtoChanged checks that the compiled proto files are
// forgotten when an imported proto file changes on disk.
func TestEditorSession_GRPCProtoChanged(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)
	dir := uriToPath(root)
	writeFile(t, filepath.Join(dir, "proto", "svc.proto"), "syntax = \"proto3\";\npackage p;\nimport \"common.proto\";\nservice S { rpc M(common.Req) returns (common.Req); }\n")
	common := filepath.Join(dir, "proto", "common.proto")
	writeFile(t, common, "syntax = \"proto3\";\npackage common;\nmessage Req { string before = 1; }\n")
	client.initialize(1, root)

	uri := file("test.yaml")
	text, line, char, _ := parseCursorMarker("schemaVersion: scenario/v1\nsteps:\n  - protocol: grpc\n    request:\n      options:\n        proto:\n          imports: [proto]\n          files: [svc.proto]\n      service: p.S\n      method: M\n      message:\n        $0\n")
	client.openDocument(uri, text)
	if labels := labelList(client.complete(2, uri, line, char).Items); !slices.Contains(labels, "before") {
		t.Fatalf("expected before, got %v", labels)
	}

	writeFile(t, common, "syntax = \"proto3\";\npackage common;\nmessage Req { string after = 1; }\n")
	client.watchedFilesChanged(FileEvent{URI: pathToURI(common), Type: FileChangeTypeChanged})
	client.sync(3)
	if labels := labelList(client.complete(4, uri, line, char).Items); !slices.Contains(labels, "after") {
		t.Errorf("expected the field of the changed import, got %v", labels)
	}
}
