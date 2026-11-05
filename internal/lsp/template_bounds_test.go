package lsp

import (
	"fmt"
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

func TestReadScannedFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, size int) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if _, ok := readScannedFile(write("limit", maxScannedFileSize)); !ok {
		t.Error("a file at the limit was not read")
	}
	if _, ok := readScannedFile(write("over", maxScannedFileSize+1)); ok {
		t.Error("a file over the limit was read")
	}
	if _, ok := readScannedFile(dir); ok {
		t.Error("a directory was read")
	}
	if _, ok := readScannedFile(filepath.Join(dir, "absent")); ok {
		t.Error("an absent file was read")
	}
}

func TestDiagnostics_NestedAliasesCheckEachAnchorOnce(t *testing.T) {
	var b strings.Builder
	b.WriteString("schemaVersion: scenario/v1\nanchors:\n  a0: &a0 ['{{vars.}}']\n")
	const depth = 32
	for i := 1; i < depth; i++ {
		fmt.Fprintf(&b, "  a%d: &a%d [*a%d, *a%d]\n", i, i, i-1, i-1)
	}
	fmt.Fprintf(&b, "vars:\n  v: *a%d\n", depth-1)
	if diags := diagnosticsWithin(t, 10*time.Second, b.String()); len(diags) != 1 {
		t.Errorf("expected one diagnostic, got %v", diagMessages(diags))
	}
}

func TestDiagnostics_AliasedStepsAreCheckedOnce(t *testing.T) {
	const n = 1000
	var vars strings.Builder
	for i := range n {
		fmt.Fprintf(&vars, "      v%d: '{{x%d}}'\n", i, i)
	}
	head := "schemaVersion: scenario/v1\nanchors:\n  s: &s\n    vars:\n" + vars.String() + "steps:\n"
	for name, step := range map[string]string{"alias": "  - *s\n", "merge": "  - <<: *s\n"} {
		t.Run(name, func(t *testing.T) {
			diags := diagnosticsWithin(t, 10*time.Second, head+strings.Repeat(step, n))
			if len(diags) != n {
				t.Errorf("expected %d diagnostics, got %d", n, len(diags))
			}
		})
	}
}

func diagnosticsWithin(t *testing.T, limit time.Duration, text string) []Diagnostic {
	t.Helper()
	client := newRunningTestClient(t)
	done := make(chan PublishDiagnosticsParams, 1)
	go func() { done <- client.openDocumentAndGetDiagnostics("file:///test.yaml", text) }()
	select {
	case diags := <-done:
		return diags.Diagnostics
	case <-time.After(limit):
		t.Fatal("the diagnostics did not finish")
		return nil
	}
}

func TestDiagnostics_TagChainDoesNotEndTheServer(t *testing.T) {
	text := "schemaVersion: scenario/v1\nvars:\n  a: " + strings.Repeat("!!str ", 1_000_000) + "1\n"
	if diags := diagnosticsWithin(t, 30*time.Second, text); len(diags) != 1 {
		t.Errorf("expected the syntax error, got %v", diagMessages(diags))
	}
}

func TestDefinition_IncludeThatIsNotARegularFile(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "fifo.yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := &Server{docs: newDocumentStore()}
	done := make(chan string, 1)
	go func() { done <- server.textOf(pathToURI(filepath.Join(root, "fifo.yaml"))) }()
	select {
	case text := <-done:
		if text != "" {
			t.Errorf("got %q", text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reading a FIFO blocked the server")
	}
}
