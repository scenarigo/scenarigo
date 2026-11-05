package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDiagnostics_DeepTemplateDoesNotEndTheServer(t *testing.T) {
	for name, template := range map[string]string{
		"parens": "{{" + strings.Repeat("(", 2_000_000),
		"unary":  "{{" + strings.Repeat("-", 2_000_000) + "1}}",
	} {
		t.Run(name, func(t *testing.T) {
			client := newRunningTestClient(t)
			diags := client.openDocumentAndGetDiagnostics("file:///test.yaml", "schemaVersion: scenario/v1\nvars:\n  a: '"+template+"'\n")
			if len(diags.Diagnostics) != 0 {
				t.Errorf("expected no diagnostics, got %v", diagMessages(diags.Diagnostics))
			}
		})
	}
}

func TestDiagnostics_ConfigThatIsNotARegularFile(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "scenarigo.yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := newRunningTestClient(t)
	client.initialize(1, pathToURI(root))
	done := make(chan PublishDiagnosticsParams, 1)
	go func() {
		done <- client.openDocumentAndGetDiagnostics(pathToURI(filepath.Join(root, "a.yaml")), "schemaVersion: scenario/v1\nvars:\n  a: 1\n")
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("reading scenarigo.yaml blocked the server")
	}
}

func TestDiagnostics_OversizedConfigIsNotRead(t *testing.T) {
	root := t.TempDir()
	config := "schemaVersion: config/v1\nvars:\n  v: 1\n" + strings.Repeat("# padding\n", maxScannedFileSize/10)
	if err := os.WriteFile(filepath.Join(root, "scenarigo.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client := newRunningTestClient(t)
	client.initialize(1, pathToURI(root))
	diags := client.openDocumentAndGetDiagnostics(pathToURI(filepath.Join(root, "a.yaml")), "schemaVersion: scenario/v1\nvars:\n  a: '{{vars.v}}'\n")
	if len(diags.Diagnostics) != 1 {
		t.Errorf("expected vars.v undefined, got %v", diagMessages(diags.Diagnostics))
	}
}
