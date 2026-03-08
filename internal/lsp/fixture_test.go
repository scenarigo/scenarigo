package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// lspTestCase represents a single LSP test case loaded from a YAML fixture file.
type lspTestCase struct {
	Name      string            `yaml:"name"`
	Document  string            `yaml:"document"`
	Operation string            `yaml:"operation"` // completion, diagnostics, definition
	Files     map[string]string `yaml:"files"`
	Expect    lspExpect         `yaml:"expect"`
}

type lspExpect struct {
	CompletionLabels   *labelMatcher   `yaml:"completionLabels"`
	DiagnosticMessages *messageMatcher `yaml:"diagnosticMessages"`
	DiagnosticCount    *int            `yaml:"diagnosticCount"`
	DefinitionURI      *uriMatcher     `yaml:"definitionURI"`
	HoverContains      []string        `yaml:"hoverContains"`
	HoverIsNull        *bool           `yaml:"hoverIsNull"`
	SymbolNames        *labelMatcher   `yaml:"symbolNames"`
	FormattedText      *string         `yaml:"formattedText"`
	SignatureLabel     *string         `yaml:"signatureLabel"`
	SignatureIsNull    *bool           `yaml:"signatureIsNull"`
	ReferenceCount    *int            `yaml:"referenceCount"`
}

type labelMatcher struct {
	Contains []string `yaml:"contains"`
	Excludes []string `yaml:"excludes"`
}

type messageMatcher struct {
	Contains []string `yaml:"contains"`
	Excludes []string `yaml:"excludes"`
}

type uriMatcher struct {
	Suffix string `yaml:"suffix"`
}

// parseCursorMarker finds and removes the $0 cursor marker from the document text.
// Returns the cleaned text, 0-based line, 0-based character, and whether a marker was found.
func parseCursorMarker(doc string) (text string, line, char int, found bool) {
	idx := strings.Index(doc, "$0")
	if idx < 0 {
		return doc, 0, 0, false
	}

	text = doc[:idx] + doc[idx+2:]

	// Calculate line and character from the byte offset.
	prefix := doc[:idx]
	line = strings.Count(prefix, "\n")
	lastNewline := strings.LastIndex(prefix, "\n")
	if lastNewline < 0 {
		char = len(prefix)
	} else {
		char = len(prefix) - lastNewline - 1
	}
	return text, line, char, true
}

// loadFixtures reads all YAML documents from the given file, each separated by "---".
func loadFixtures(t *testing.T, path string) []lspTestCase {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture file %s: %v", path, err)
	}

	var cases []lspTestCase
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var tc lspTestCase
		if err := dec.Decode(&tc); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("decode fixture %s: %v", path, err)
		}
		if tc.Name == "" {
			continue
		}
		cases = append(cases, tc)
	}
	return cases
}

// runLSPTestFixtures runs all test cases from the given fixture file.
func runLSPTestFixtures(t *testing.T, fixturePath string) {
	t.Helper()
	cases := loadFixtures(t, fixturePath)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			runSingleFixture(t, tc)
		})
	}
}

func runSingleFixture(t *testing.T, tc lspTestCase) {
	t.Helper()

	docText, cursorLine, cursorChar, hasCursor := parseCursorMarker(tc.Document)

	switch tc.Operation {
	case "completion":
		if !hasCursor {
			t.Fatal("completion test requires $0 cursor marker in document")
		}
		runCompletionFixture(t, tc, docText, cursorLine, cursorChar)
	case "diagnostics":
		runDiagnosticsFixture(t, tc, docText)
	case "hover":
		if !hasCursor {
			t.Fatal("hover test requires $0 cursor marker in document")
		}
		runHoverFixture(t, tc, docText, cursorLine, cursorChar)
	case "documentSymbol":
		runDocumentSymbolFixture(t, tc, docText)
	case "definition":
		if !hasCursor {
			t.Fatal("definition test requires $0 cursor marker in document")
		}
		runDefinitionFixture(t, tc, docText, cursorLine, cursorChar)
	case "formatting":
		runFormattingFixture(t, tc, docText)
	case "signatureHelp":
		if !hasCursor {
			t.Fatal("signatureHelp test requires $0 cursor marker in document")
		}
		runSignatureHelpFixture(t, tc, docText, cursorLine, cursorChar)
	case "references":
		if !hasCursor {
			t.Fatal("references test requires $0 cursor marker in document")
		}
		runReferencesFixture(t, tc, docText, cursorLine, cursorChar)
	default:
		t.Fatalf("unknown operation: %s", tc.Operation)
	}
}

func runCompletionFixture(t *testing.T, tc lspTestCase, docText string, line, char int) {
	t.Helper()

	// Use a temp dir when auxiliary files are specified (e.g. file path completion).
	rootDir := "/tmp"
	if len(tc.Files) > 0 {
		rootDir = t.TempDir()
		for name, content := range tc.Files {
			p := filepath.Join(rootDir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatalf("write file %s: %v", name, err)
			}
		}
	}

	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	rootURI := "file://" + rootDir
	client.initialize(1, rootURI)

	uri := "file://" + rootDir + "/test.yaml"
	client.openDocument(uri, docText)

	list := client.complete(2, uri, line, char)

	if tc.Expect.CompletionLabels != nil {
		labels := labelSet(list.Items)
		for _, want := range tc.Expect.CompletionLabels.Contains {
			if !labels[want] {
				t.Errorf("expected %q in completions, got: %v", want, labelList(list.Items))
			}
		}
		for _, exclude := range tc.Expect.CompletionLabels.Excludes {
			if labels[exclude] {
				t.Errorf("should not have %q in completions", exclude)
			}
		}
	}
}

func runDiagnosticsFixture(t *testing.T, tc lspTestCase, docText string) {
	t.Helper()

	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	uri := "file:///tmp/test.yaml"
	diags := client.openDocumentAndGetDiagnostics(uri, docText)

	if tc.Expect.DiagnosticCount != nil {
		if len(diags.Diagnostics) != *tc.Expect.DiagnosticCount {
			t.Errorf("expected %d diagnostics, got %d: %v", *tc.Expect.DiagnosticCount, len(diags.Diagnostics), diagMessages(diags.Diagnostics))
		}
	}

	if tc.Expect.DiagnosticMessages != nil {
		msgs := make(map[string]bool)
		for _, d := range diags.Diagnostics {
			msgs[d.Message] = true
		}
		for _, want := range tc.Expect.DiagnosticMessages.Contains {
			if !msgs[want] {
				t.Errorf("expected diagnostic %q, got: %v", want, diagMessages(diags.Diagnostics))
			}
		}
		for _, exclude := range tc.Expect.DiagnosticMessages.Excludes {
			if msgs[exclude] {
				t.Errorf("should not have diagnostic %q", exclude)
			}
		}
	}
}

func runDocumentSymbolFixture(t *testing.T, tc lspTestCase, docText string) {
	t.Helper()

	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	uri := "file:///tmp/test.yaml"
	client.openDocument(uri, docText)

	resp := client.documentSymbol(2, uri)

	if tc.Expect.SymbolNames != nil {
		var symbols []DocumentSymbol
		if err := json.Unmarshal(resp, &symbols); err != nil {
			t.Fatalf("unmarshal symbols: %v", err)
		}
		names := collectSymbolNames(symbols)
		for _, want := range tc.Expect.SymbolNames.Contains {
			if !names[want] {
				t.Errorf("expected symbol %q, got: %v", want, symbolNameList(symbols))
			}
		}
		for _, exclude := range tc.Expect.SymbolNames.Excludes {
			if names[exclude] {
				t.Errorf("should not have symbol %q", exclude)
			}
		}
	}
}

func collectSymbolNames(symbols []DocumentSymbol) map[string]bool {
	names := make(map[string]bool)
	var walk func([]DocumentSymbol)
	walk = func(syms []DocumentSymbol) {
		for _, s := range syms {
			names[s.Name] = true
			walk(s.Children)
		}
	}
	walk(symbols)
	return names
}

func symbolNameList(symbols []DocumentSymbol) []string {
	var names []string
	var walk func([]DocumentSymbol)
	walk = func(syms []DocumentSymbol) {
		for _, s := range syms {
			names = append(names, s.Name)
			walk(s.Children)
		}
	}
	walk(symbols)
	return names
}

func runHoverFixture(t *testing.T, tc lspTestCase, docText string, line, char int) {
	t.Helper()

	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	uri := "file:///tmp/test.yaml"
	client.openDocument(uri, docText)

	resp := client.hover(2, uri, line, char)

	if tc.Expect.HoverIsNull != nil && *tc.Expect.HoverIsNull {
		if string(resp) != "null" {
			t.Errorf("expected null hover, got: %s", resp)
		}
		return
	}

	if len(tc.Expect.HoverContains) > 0 {
		var hoverResult Hover
		if err := json.Unmarshal(resp, &hoverResult); err != nil {
			t.Fatalf("unmarshal hover: %v", err)
		}
		for _, want := range tc.Expect.HoverContains {
			if !strings.Contains(hoverResult.Contents.Value, want) {
				t.Errorf("expected hover to contain %q, got: %s", want, hoverResult.Contents.Value)
			}
		}
	}
}

func runDefinitionFixture(t *testing.T, tc lspTestCase, docText string, line, char int) {
	t.Helper()

	tmpDir := t.TempDir()

	// Create auxiliary files.
	for name, content := range tc.Files {
		p := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write file %s: %v", name, err)
		}
	}

	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	rootURI := fmt.Sprintf("file://%s", tmpDir)
	client.initialize(1, rootURI)

	docURI := "file://" + filepath.Join(tmpDir, "test.yaml")
	client.openDocument(docURI, docText)

	resp := client.definition(2, docURI, line, char)

	if tc.Expect.DefinitionURI != nil {
		var loc Location
		if err := json.Unmarshal(resp, &loc); err != nil {
			t.Fatalf("unmarshal location: %v", err)
		}
		if tc.Expect.DefinitionURI.Suffix != "" {
			if !strings.HasSuffix(loc.URI, tc.Expect.DefinitionURI.Suffix) {
				t.Errorf("expected URI ending with %q, got: %s", tc.Expect.DefinitionURI.Suffix, loc.URI)
			}
		}
	}
}

func runFormattingFixture(t *testing.T, tc lspTestCase, docText string) {
	t.Helper()

	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	uri := "file:///tmp/test.yaml"
	client.openDocument(uri, docText)

	resp := client.formatting(2, uri)

	if tc.Expect.FormattedText != nil {
		var edits []TextEdit
		if string(resp) != "null" {
			if err := json.Unmarshal(resp, &edits); err != nil {
				t.Fatalf("unmarshal text edits: %v", err)
			}
		}
		result := applyTextEdits(docText, edits)
		if result != *tc.Expect.FormattedText {
			t.Errorf("formatted text mismatch:\ngot:\n%s\nwant:\n%s", result, *tc.Expect.FormattedText)
		}
	}
}

// applyTextEdits applies LSP text edits to the original text.
func applyTextEdits(text string, edits []TextEdit) string {
	if len(edits) == 0 {
		return text
	}
	lines := strings.Split(text, "\n")

	// Apply edits in reverse order to preserve positions.
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		startOff := lineCharToOffset(lines, e.Range.Start.Line, e.Range.Start.Character)
		endOff := lineCharToOffset(lines, e.Range.End.Line, e.Range.End.Character)
		text = text[:startOff] + e.NewText + text[endOff:]
		lines = strings.Split(text, "\n")
	}
	return text
}

func lineCharToOffset(lines []string, line, char int) int {
	off := 0
	for i := 0; i < line && i < len(lines); i++ {
		off += len(lines[i]) + 1 // +1 for \n
	}
	off += char
	return off
}

func runSignatureHelpFixture(t *testing.T, tc lspTestCase, docText string, line, char int) {
	t.Helper()

	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	uri := "file:///tmp/test.yaml"
	client.openDocument(uri, docText)

	resp := client.signatureHelp(2, uri, line, char)

	if tc.Expect.SignatureIsNull != nil && *tc.Expect.SignatureIsNull {
		if string(resp) != "null" {
			t.Errorf("expected null signature help, got: %s", resp)
		}
		return
	}

	if tc.Expect.SignatureLabel != nil {
		var help SignatureHelp
		if err := json.Unmarshal(resp, &help); err != nil {
			t.Fatalf("unmarshal signature help: %v", err)
		}
		if len(help.Signatures) == 0 {
			t.Fatal("expected at least one signature")
		}
		if help.Signatures[0].Label != *tc.Expect.SignatureLabel {
			t.Errorf("expected signature label %q, got %q", *tc.Expect.SignatureLabel, help.Signatures[0].Label)
		}
	}
}

func runReferencesFixture(t *testing.T, tc lspTestCase, docText string, line, char int) {
	t.Helper()

	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	client.initialize(1, "file:///tmp")

	uri := "file:///tmp/test.yaml"
	client.openDocument(uri, docText)

	resp := client.references(2, uri, line, char)

	if tc.Expect.ReferenceCount != nil {
		var locs []Location
		if string(resp) != "null" {
			if err := json.Unmarshal(resp, &locs); err != nil {
				t.Fatalf("unmarshal locations: %v", err)
			}
		}
		if len(locs) != *tc.Expect.ReferenceCount {
			t.Errorf("expected %d references, got %d: %v", *tc.Expect.ReferenceCount, len(locs), locs)
		}
	}
}
