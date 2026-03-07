package lsp

import (
	"bytes"
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
	case "definition":
		if !hasCursor {
			t.Fatal("definition test requires $0 cursor marker in document")
		}
		runDefinitionFixture(t, tc, docText, cursorLine, cursorChar)
	default:
		t.Fatalf("unknown operation: %s", tc.Operation)
	}
}

func runCompletionFixture(t *testing.T, tc lspTestCase, docText string, line, char int) {
	t.Helper()

	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	uri := "file:///tmp/test.yaml"
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
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

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

func runHoverFixture(t *testing.T, tc lspTestCase, docText string, line, char int) {
	t.Helper()

	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	uri := "file:///tmp/test.yaml"
	client.openDocument(uri, docText)

	client.sendRequest(2, "textDocument/hover", fmt.Sprintf(`{
		"textDocument": {"uri": %q},
		"position": {"line": %d, "character": %d}
	}`, uri, line, char))
	resp := client.readResponse()

	if tc.Expect.HoverIsNull != nil && *tc.Expect.HoverIsNull {
		if string(resp) != "null" {
			t.Errorf("expected null hover, got: %s", resp)
		}
		return
	}

	if len(tc.Expect.HoverContains) > 0 {
		var hover Hover
		if err := json.Unmarshal(resp, &hover); err != nil {
			t.Fatalf("unmarshal hover: %v", err)
		}
		for _, want := range tc.Expect.HoverContains {
			if !strings.Contains(hover.Contents.Value, want) {
				t.Errorf("expected hover to contain %q, got: %s", want, hover.Contents.Value)
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
	go srv.Run()

	client.sendRequest(1, "initialize", fmt.Sprintf(`{"rootUri":"file://%s"}`, tmpDir))
	client.readResponse()

	docURI := "file://" + filepath.Join(tmpDir, "test.yaml")
	client.openDocument(docURI, docText)

	client.sendRequest(2, "textDocument/definition", fmt.Sprintf(`{
		"textDocument": {"uri": %q},
		"position": {"line": %d, "character": %d}
	}`, docURI, line, char))
	resp := client.readResponse()

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
