package lsp

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestEditorSession_OpenEditComplete(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

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
	client.sendNotification("textDocument/didChange", fmt.Sprintf(`{
		"textDocument": {"uri": "file:///tmp/test.yaml", "version": 2},
		"contentChanges": [{"text": %s}]
	}`, jsonString(newText)))
	time.Sleep(10 * time.Millisecond)
	client.readMessage() // diagnostics after change

	// Completions should now show request fields.
	list = client.complete(3, "file:///tmp/test.yaml", 6, 6)
	labels = labelSet(list.Items)
	if !labels["method"] {
		t.Errorf("expected 'method' after edit, got: %v", labelList(list.Items))
	}
}

func TestEditorSession_MultipleDocuments(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

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
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	// Open valid document.
	docText := "schemaVersion: scenario/v1\ntitle: test\n"
	diags := client.openDocumentAndGetDiagnostics("file:///tmp/test.yaml", docText)
	if len(diags.Diagnostics) > 0 {
		t.Errorf("expected no diagnostics for valid document, got: %v", diagMessages(diags.Diagnostics))
	}

	// Edit to introduce unknown key.
	badText := "schemaVersion: scenario/v1\ntitle: test\nbadKey: value\n"
	client.sendNotification("textDocument/didChange", fmt.Sprintf(`{
		"textDocument": {"uri": "file:///tmp/test.yaml", "version": 2},
		"contentChanges": [{"text": %s}]
	}`, jsonString(badText)))
	time.Sleep(10 * time.Millisecond)
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
	go srv.Run()

	// Initialize.
	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	result := client.readResponse()
	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		t.Fatalf("unmarshal init result: %v", err)
	}
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
	client.sendRequest(3, "textDocument/hover", `{
		"textDocument": {"uri": "file:///tmp/test.yaml"},
		"position": {"line": 4, "character": 5}
	}`)
	client.readResponse()

	// Close document.
	client.sendNotification("textDocument/didClose", `{
		"textDocument": {"uri": "file:///tmp/test.yaml"}
	}`)
	time.Sleep(10 * time.Millisecond)
	client.readMessage() // clear diagnostics notification

	// Shutdown.
	client.sendRequest(4, "shutdown", `{}`)
	client.readResponse()
}
