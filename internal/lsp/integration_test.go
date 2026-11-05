package lsp

import (
	"encoding/json"
	"testing"
)

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
