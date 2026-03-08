//go:build e2e_lsp

package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var e2eBinary string

func TestMain(m *testing.M) {
	root := findModuleRoot()

	// Build the binary using `make build` (installs to .bin/scenarigo).
	cmd := exec.Command("make", "build")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "make build: %v\n%s\n", err, out)
		os.Exit(1)
	}
	e2eBinary = filepath.Join(root, ".bin", "scenarigo")

	os.Exit(m.Run())
}

// findModuleRoot walks up from this file's directory to find the go.mod.
func findModuleRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("could not find module root (go.mod)")
		}
		dir = parent
	}
}

// startServer launches the LSP server binary as a subprocess and returns
// a testClient connected to its stdin/stdout.
func startServer(t *testing.T, args ...string) *testClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	cmdArgs := append([]string{"lsp"}, args...)
	cmd := exec.CommandContext(ctx, e2eBinary, cmdArgs...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start server: %v", err)
	}

	t.Cleanup(func() {
		stdin.Close()
		cancel()
		_ = cmd.Wait()
	})

	return &testClient{
		t:    t,
		inW:  stdin,
		outR: bufio.NewReader(stdout),
	}
}

// TestE2E_FullLifecycle tests a realistic editor session through the real binary:
// initialize → open → diagnostics → complete → hover → symbols → shutdown → exit.
func TestE2E_FullLifecycle(t *testing.T) {
	client := startServer(t)

	// Initialize.
	initResult := client.initialize(1, "file:///tmp")
	if initResult.Capabilities.CompletionProvider == nil {
		t.Fatal("expected completion provider")
	}
	if !initResult.Capabilities.HoverProvider {
		t.Fatal("expected hover provider")
	}

	// Open document and get diagnostics.
	docText := "schemaVersion: scenario/v1\ntitle: e2e test\nsteps:\n  - title: step1\n    protocol: http\n    request:\n      method: GET\n      url: http://example.com\n"
	diags := client.openDocumentAndGetDiagnostics("file:///tmp/test.yaml", docText)
	if len(diags.Diagnostics) != 0 {
		t.Errorf("expected no diagnostics for valid document, got: %v", diagMessages(diags.Diagnostics))
	}

	// Completion.
	list := client.complete(2, "file:///tmp/test.yaml", 8, 6)
	_ = list // verify no error

	// Hover.
	hoverResp := client.hover(3, "file:///tmp/test.yaml", 4, 5)
	var hoverResult Hover
	if err := json.Unmarshal(hoverResp, &hoverResult); err != nil {
		t.Fatalf("unmarshal hover: %v", err)
	}

	// Document symbols.
	symResp := client.documentSymbol(4, "file:///tmp/test.yaml")
	var symbols []DocumentSymbol
	if err := json.Unmarshal(symResp, &symbols); err != nil {
		t.Fatalf("unmarshal symbols: %v", err)
	}
	if len(symbols) == 0 {
		t.Error("expected at least one document symbol")
	}

	// Shutdown.
	client.shutdown(5)
}

// TestE2E_ShutdownExit verifies the server exits cleanly after shutdown + exit.
func TestE2E_ShutdownExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, e2eBinary, "lsp")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}

	client := &testClient{t: t, inW: stdin, outR: bufio.NewReader(stdout)}

	// Initialize + shutdown.
	client.initialize(1, "file:///tmp")
	client.shutdown(2)

	// Send exit notification.
	client.sendNotification("exit", struct{}{})

	// Wait for the process to exit.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("server exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not exit within 5 seconds after exit notification")
	}
}

// TestE2E_StdinClose verifies the server exits gracefully when stdin is closed.
func TestE2E_StdinClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, e2eBinary, "lsp")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if _, err := cmd.StdoutPipe(); err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}

	// Close stdin immediately.
	stdin.Close()

	// The server should exit.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		// Exited (any exit code is acceptable here).
	case <-time.After(5 * time.Second):
		t.Fatal("server did not exit within 5 seconds after stdin close")
	}
}

// TestE2E_DiagnosticsOnEdit verifies diagnostics are published when a document is edited.
func TestE2E_DiagnosticsOnEdit(t *testing.T) {
	client := startServer(t)

	client.initialize(1, "file:///tmp")

	// Open valid document.
	diags := client.openDocumentAndGetDiagnostics("file:///tmp/test.yaml",
		"schemaVersion: scenario/v1\ntitle: test\n")
	if len(diags.Diagnostics) != 0 {
		t.Errorf("expected no diagnostics initially, got: %v", diagMessages(diags.Diagnostics))
	}

	// Edit to introduce unknown key — read the diagnostics notification manually.
	badText := "schemaVersion: scenario/v1\ntitle: test\nbadKey: value\n"
	client.sendNotification("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: TextDocumentIdentifier{URI: "file:///tmp/test.yaml"},
			Version:                2,
		},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: badText}},
	})

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
		t.Errorf("expected diagnostic for badKey, got: %v", diagMessages(updatedDiags.Diagnostics))
	}
}

// TestE2E_ConfigVarsFromDisk verifies that vars from a real scenarigo.yaml on disk
// are available in template completion.
func TestE2E_ConfigVarsFromDisk(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a config file.
	configContent := "schemaVersion: config/v1\nvars:\n  e2eVar: hello\n  anotherE2EVar: world\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "scenarigo.yaml"), []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	client := startServer(t)

	rootURI := "file://" + tmpDir
	client.initialize(1, rootURI)

	// Open a scenario that uses {{vars.
	docText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    request:\n      url: '{{vars.}}'\n"
	uri := "file://" + filepath.Join(tmpDir, "test.yaml")
	client.openDocument(uri, docText)

	list := client.complete(2, uri, 5, 19)
	labels := labelSet(list.Items)
	if !labels["e2eVar"] || !labels["anotherE2EVar"] {
		t.Errorf("expected config vars in completion, got: %v", labelList(list.Items))
	}
}

// TestE2E_FormattingRoundTrip verifies formatting works through the real binary.
func TestE2E_FormattingRoundTrip(t *testing.T) {
	client := startServer(t)

	client.initialize(1, "file:///tmp")

	// Open a document with unordered keys.
	docText := "schemaVersion: scenario/v1\nsteps:\n  - title: step1\n    protocol: http\ntitle: test\n"
	client.openDocument("file:///tmp/test.yaml", docText)

	resp := client.formatting(2, "file:///tmp/test.yaml")

	var edits []TextEdit
	if string(resp) != "null" {
		if err := json.Unmarshal(resp, &edits); err != nil {
			t.Fatalf("unmarshal text edits: %v", err)
		}
	}
	// We just verify it doesn't error — the exact formatting is tested in unit tests.
	// But if edits are returned, the result should be valid.
	if len(edits) > 0 {
		result := applyTextEdits(docText, edits)
		if result == "" {
			t.Error("formatting produced empty result")
		}
	}
}

// TestE2E_Neovim runs a full LSP session inside headless Neovim.
// This tests the real editor integration: LSP client attach, completion,
// hover, diagnostics, and diagnostics-after-edit — all driven by Neovim's
// built-in LSP client with vim.lsp.start.
func TestE2E_Neovim(t *testing.T) {
	// Check that nvim is available.
	nvimPath, err := exec.LookPath("nvim")
	if err != nil {
		t.Skip("nvim not found in PATH; skipping Neovim e2e test")
	}

	// Verify minimum version (0.10+ for vim.lsp.get_clients).
	out, err := exec.Command(nvimPath, "--version").Output()
	if err != nil {
		t.Skipf("failed to get nvim version: %v", err)
	}
	firstLine := strings.SplitN(string(out), "\n", 2)[0] // "NVIM v0.11.0"
	if ver := strings.TrimPrefix(firstLine, "NVIM v"); ver != firstLine {
		parts := strings.SplitN(ver, ".", 3)
		if len(parts) >= 2 {
			major := parts[0]
			minor := parts[1]
			if major == "0" && minor < "10" {
				t.Skipf("nvim %s is too old (need >= 0.10); skipping", ver)
			}
		}
	}

	workDir := t.TempDir()

	// Locate the Lua test script.
	luaScript := filepath.Join(findModuleRoot(), "internal", "lsp", "testdata", "e2e_nvim", "lsp_test.lua")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, nvimPath,
		"--headless",
		"--clean",
		"-u", "NONE",
		"-S", luaScript,
	)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"HOME="+workDir,                    // isolate from user config
		"XDG_CONFIG_HOME="+workDir,         // no user plugins
		"XDG_DATA_HOME="+workDir,           // no shared data
		"XDG_STATE_HOME="+workDir,          // no state files
		"SCENARIGO_BINARY="+e2eBinary,      // path to LSP server binary
		"SCENARIGO_WORKDIR="+workDir,       // workspace directory
	)

	output, err := cmd.CombinedOutput()

	// Parse results.
	resultsPath := filepath.Join(workDir, "results.json")
	resultsData, readErr := os.ReadFile(resultsPath)
	if readErr != nil {
		t.Logf("nvim output:\n%s", output)
		if err != nil {
			t.Fatalf("nvim exited with error: %v", err)
		}
		t.Fatalf("failed to read results.json: %v", readErr)
	}

	var results struct {
		Tests []struct {
			Name   string `json:"name"`
			OK     bool   `json:"ok"`
			Detail string `json:"detail"`
		} `json:"tests"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(resultsData, &results); err != nil {
		t.Logf("nvim output:\n%s", output)
		t.Fatalf("failed to parse results.json: %v", err)
	}

	// Report errors.
	for _, e := range results.Errors {
		t.Errorf("nvim error: %s", e)
	}

	// Report individual test results.
	for _, test := range results.Tests {
		t.Run("nvim/"+test.Name, func(t *testing.T) {
			if !test.OK {
				t.Errorf("FAIL: %s (%s)", test.Name, test.Detail)
			} else {
				t.Logf("PASS: %s (%s)", test.Name, test.Detail)
			}
		})
	}

	if len(results.Tests) == 0 {
		t.Logf("nvim output:\n%s", output)
		t.Error("no test results from Neovim")
	}
}
