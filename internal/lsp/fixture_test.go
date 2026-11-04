package lsp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/goccy/go-yaml"
)

// lspTestCase represents a single LSP test case loaded from a YAML fixture file.
type lspTestCase struct {
	Name      string            `yaml:"name"`
	Document  string            `yaml:"document"`
	Operation string            `yaml:"operation"` // see runSingleFixture for the supported operations
	Files     map[string]string `yaml:"files"`
	NewName   string            `yaml:"newName"` // rename
	Expect    lspExpect         `yaml:"expect"`
}

type lspExpect struct {
	CompletionLabels *labelMatcher `yaml:"completionLabels"`
	// CompletionEdits lists the text edits of completion items by label;
	// the range is "line:start-end" (0-based, UTF-16).
	CompletionEdits    []completionEdit `yaml:"completionEdits"`
	DiagnosticMessages *messageMatcher  `yaml:"diagnosticMessages"`
	Diagnostics        []diagMatcher    `yaml:"diagnostics"`
	DiagnosticCount    *int             `yaml:"diagnosticCount"`
	DefinitionURI      *uriMatcher      `yaml:"definitionUri"`
	DefinitionIsNull   *bool            `yaml:"definitionIsNull"`
	HoverContains      []string         `yaml:"hoverContains"`
	HoverIsNull        *bool            `yaml:"hoverIsNull"`
	SymbolNames        *labelMatcher    `yaml:"symbolNames"`
	SignatureLabel     *string          `yaml:"signatureLabel"`
	SignatureIsNull    *bool            `yaml:"signatureIsNull"`
	ReferenceCount     *int             `yaml:"referenceCount"`
	// RenameEdits lists the ranges a rename replaces in the document, as
	// "line:start-end" (0-based, UTF-16), in document order.
	RenameEdits  []string `yaml:"renameEdits"`
	RenameError  string   `yaml:"renameError"`  // substring of the error message
	PrepareRange string   `yaml:"prepareRange"` // "line:start-end"
	PrepareNull  bool     `yaml:"prepareIsNull"`
	PrepareError string   `yaml:"prepareError"` // substring of the error message
	// CodeLenses lists the lenses as "line:title:command".
	CodeLenses *[]string `yaml:"codeLenses"`
}

type completionEdit struct {
	Label   string `yaml:"label"`
	Range   string `yaml:"range"`
	NewText string `yaml:"newText"`
}

type labelMatcher struct {
	Contains []string `yaml:"contains"`
	Excludes []string `yaml:"excludes"`
}

type messageMatcher struct {
	Contains []string `yaml:"contains"`
	Excludes []string `yaml:"excludes"`
}

// diagMatcher matches the diagnostic with the given message. The other
// fields are checked when set; lines and characters are 0-based, and
// characters count UTF-16 code units like the test client.
type diagMatcher struct {
	Message   string `yaml:"message"`
	Severity  *int   `yaml:"severity"`
	StartLine *int   `yaml:"startLine"`
	StartChar *int   `yaml:"startChar"`
	EndLine   *int   `yaml:"endLine"`
	EndChar   *int   `yaml:"endChar"`
}

type uriMatcher struct {
	Suffix    string `yaml:"suffix"`
	StartLine *int   `yaml:"startLine"`
	StartChar *int   `yaml:"startChar"`
	EndLine   *int   `yaml:"endLine"`
	EndChar   *int   `yaml:"endChar"`
}

// parseCursorMarker finds and removes the $0 cursor marker from the document text.
// Returns the cleaned text, 0-based line, 0-based character, and whether a marker was found.
func parseCursorMarker(doc string) (string, int, int, bool) {
	before, after, ok := strings.Cut(doc, "$0")
	if !ok {
		return doc, 0, 0, false
	}

	text := before + after

	// Calculate the line from the byte offset. The character is counted in
	// UTF-16 code units: the test client offers no encoding in initialize,
	// so the server falls back to the LSP default.
	prefix := before
	line := strings.Count(prefix, "\n")
	char := len(utf16.Encode([]rune(prefix[strings.LastIndex(prefix, "\n")+1:])))
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
			if errors.Is(err, io.EOF) {
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
	case "codeLens":
		runCodeLensFixture(t, tc, docText)
	case "rename", "prepareRename":
		if !hasCursor {
			t.Fatalf("%s test requires $0 cursor marker in document", tc.Operation)
		}
		runRenameFixture(t, tc, docText, cursorLine, cursorChar)
	default:
		t.Fatalf("unknown operation: %s", tc.Operation)
	}
}

func runCompletionFixture(t *testing.T, tc lspTestCase, docText string, line, char int) {
	t.Helper()

	// Use a temp dir when auxiliary files are specified (e.g. file path completion).
	rootDir := t.TempDir()
	if len(tc.Files) > 0 {
		for name, content := range tc.Files {
			p := filepath.Join(rootDir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
				t.Fatalf("write file %s: %v", name, err)
			}
		}
	}

	client := newRunningTestClient(t)

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
	for _, want := range tc.Expect.CompletionEdits {
		i := slices.IndexFunc(list.Items, func(item CompletionItem) bool { return item.Label == want.Label })
		if i < 0 {
			t.Errorf("expected %q in completions, got: %v", want.Label, labelList(list.Items))
			continue
		}
		edit := list.Items[i].TextEdit
		if edit == nil {
			t.Errorf("completion %q has no text edit", want.Label)
			continue
		}
		if r := formatLineRange(edit.Range); r != want.Range || edit.NewText != want.NewText {
			t.Errorf("completion %q edits %s with %q, want %s with %q", want.Label, r, edit.NewText, want.Range, want.NewText)
		}
	}
}

func runDiagnosticsFixture(t *testing.T, tc lspTestCase, docText string) {
	t.Helper()

	client := newRunningTestClient(t)

	root, file := newWorkspace(t)
	writeFixtureFiles(t, uriToPath(root), tc.Files)
	client.initialize(1, root)

	uri := file("test.yaml")
	diags := client.openDocumentAndGetDiagnostics(uri, docText)

	if tc.Expect.DiagnosticCount != nil {
		if len(diags.Diagnostics) != *tc.Expect.DiagnosticCount {
			t.Errorf("expected %d diagnostics, got %d: %v", *tc.Expect.DiagnosticCount, len(diags.Diagnostics), diagMessages(diags.Diagnostics))
		}
	}

	for _, want := range tc.Expect.Diagnostics {
		matchDiagnostic(t, want, diags.Diagnostics)
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

// writeFixtureFiles creates the auxiliary files of a fixture under dir.
func writeFixtureFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("write file %s: %v", name, err)
		}
	}
}

func matchDiagnostic(t *testing.T, want diagMatcher, diags []Diagnostic) {
	t.Helper()
	idx := slices.IndexFunc(diags, func(d Diagnostic) bool { return d.Message == want.Message })
	if idx < 0 {
		t.Errorf("expected diagnostic %q, got: %v", want.Message, diagMessages(diags))
		return
	}
	got := diags[idx]
	for _, f := range []struct {
		name string
		want *int
		got  int
	}{
		{"severity", want.Severity, got.Severity},
		{"start line", want.StartLine, got.Range.Start.Line},
		{"start character", want.StartChar, got.Range.Start.Character},
		{"end line", want.EndLine, got.Range.End.Line},
		{"end character", want.EndChar, got.Range.End.Character},
	} {
		if f.want != nil && *f.want != f.got {
			t.Errorf("diagnostic %q: %s = %d, want %d (range %+v)", want.Message, f.name, f.got, *f.want, got.Range)
		}
	}
}

func runDocumentSymbolFixture(t *testing.T, tc lspTestCase, docText string) {
	t.Helper()

	client := newRunningTestClient(t)

	root, file := newWorkspace(t)
	client.initialize(1, root)

	uri := file("test.yaml")
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

	client := newRunningTestClient(t)

	root, file := newWorkspace(t)
	writeFixtureFiles(t, uriToPath(root), tc.Files)
	client.initialize(1, root)

	uri := file("test.yaml")
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
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("write file %s: %v", name, err)
		}
	}

	client := newRunningTestClient(t)

	rootURI := fmt.Sprintf("file://%s", tmpDir)
	client.initialize(1, rootURI)

	docURI := "file://" + filepath.Join(tmpDir, "test.yaml")
	client.openDocument(docURI, docText)

	resp := client.definition(2, docURI, line, char)

	if tc.Expect.DefinitionIsNull != nil && *tc.Expect.DefinitionIsNull {
		if string(resp) != "null" {
			t.Errorf("expected no definition, got: %s", resp)
		}
		return
	}

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
		if tc.Expect.DefinitionURI.StartLine != nil {
			if loc.Range.Start.Line != *tc.Expect.DefinitionURI.StartLine {
				t.Errorf("expected start line %d, got %d", *tc.Expect.DefinitionURI.StartLine, loc.Range.Start.Line)
			}
		}
		if tc.Expect.DefinitionURI.StartChar != nil {
			if loc.Range.Start.Character != *tc.Expect.DefinitionURI.StartChar {
				t.Errorf("expected start char %d, got %d", *tc.Expect.DefinitionURI.StartChar, loc.Range.Start.Character)
			}
		}
		if tc.Expect.DefinitionURI.EndLine != nil {
			if loc.Range.End.Line != *tc.Expect.DefinitionURI.EndLine {
				t.Errorf("expected end line %d, got %d", *tc.Expect.DefinitionURI.EndLine, loc.Range.End.Line)
			}
		}
		if tc.Expect.DefinitionURI.EndChar != nil {
			if loc.Range.End.Character != *tc.Expect.DefinitionURI.EndChar {
				t.Errorf("expected end char %d, got %d", *tc.Expect.DefinitionURI.EndChar, loc.Range.End.Character)
			}
		}
	}
}

func runSignatureHelpFixture(t *testing.T, tc lspTestCase, docText string, line, char int) {
	t.Helper()

	client := newRunningTestClient(t)

	root, file := newWorkspace(t)
	client.initialize(1, root)

	uri := file("test.yaml")
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

	client := newRunningTestClient(t)

	root, file := newWorkspace(t)
	client.initialize(1, root)

	uri := file("test.yaml")
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

func runRenameFixture(t *testing.T, tc lspTestCase, docText string, line, char int) {
	t.Helper()

	client := newRunningTestClient(t)
	root, file := newWorkspace(t)
	writeFixtureFiles(t, uriToPath(root), tc.Files)
	client.initialize(1, root)
	uri := file("test.yaml")
	client.openDocument(uri, docText)

	if tc.Operation == "prepareRename" {
		resp, respErr := client.prepareRename(2, uri, line, char)
		switch {
		case tc.Expect.PrepareError != "":
			if respErr == nil || !strings.Contains(respErr.Message, tc.Expect.PrepareError) {
				t.Errorf("expected an error containing %q, got %v (result %s)", tc.Expect.PrepareError, respErr, resp)
			}
		case respErr != nil:
			t.Fatalf("prepareRename error: %s", respErr.Message)
		case tc.Expect.PrepareNull:
			if string(resp) != "null" {
				t.Errorf("expected null, got %s", resp)
			}
		default:
			var got struct {
				Range       Range  `json:"range"`
				Placeholder string `json:"placeholder"`
			}
			if err := json.Unmarshal(resp, &got); err != nil {
				t.Fatalf("unmarshal prepareRename: %v (%s)", err, resp)
			}
			if r := formatLineRange(got.Range); r != tc.Expect.PrepareRange {
				t.Errorf("prepareRename range = %s, want %s", r, tc.Expect.PrepareRange)
			}
		}
		return
	}

	resp, respErr := client.rename(2, uri, line, char, tc.NewName)
	if tc.Expect.RenameError != "" {
		if respErr == nil || !strings.Contains(respErr.Message, tc.Expect.RenameError) {
			t.Errorf("expected an error containing %q, got %v (result %s)", tc.Expect.RenameError, respErr, resp)
		}
		return
	}
	if respErr != nil {
		t.Fatalf("rename error: %s", respErr.Message)
	}
	var edit WorkspaceEdit
	if err := json.Unmarshal(resp, &edit); err != nil {
		t.Fatalf("unmarshal workspace edit: %v (%s)", err, resp)
	}
	for u := range edit.Changes {
		if u != uri {
			t.Errorf("rename edits another file: %s", u)
		}
	}
	var got []string
	for _, e := range edit.Changes[uri] {
		if e.NewText != tc.NewName {
			t.Errorf("edit %s inserts %q, want %q", formatLineRange(e.Range), e.NewText, tc.NewName)
		}
		got = append(got, formatLineRange(e.Range))
	}
	slices.SortFunc(got, compareLineRanges)
	if !slices.Equal(got, tc.Expect.RenameEdits) {
		t.Errorf("rename edits = %v, want %v", got, tc.Expect.RenameEdits)
	}
}

// formatLineRange formats a single-line range as "line:start-end".
func formatLineRange(r Range) string {
	if r.Start.Line != r.End.Line {
		return fmt.Sprintf("%d:%d-%d:%d", r.Start.Line, r.Start.Character, r.End.Line, r.End.Character)
	}
	return fmt.Sprintf("%d:%d-%d", r.Start.Line, r.Start.Character, r.End.Character)
}

func compareLineRanges(a, b string) int {
	var al, ac, bl, bc int
	_, _ = fmt.Sscanf(a, "%d:%d", &al, &ac)
	_, _ = fmt.Sscanf(b, "%d:%d", &bl, &bc)
	if al != bl {
		return al - bl
	}
	return ac - bc
}

func runCodeLensFixture(t *testing.T, tc lspTestCase, docText string) {
	t.Helper()

	client := newRunningTestClient(t)
	root, file := newWorkspace(t)
	writeFixtureFiles(t, uriToPath(root), tc.Files)
	client.initialize(1, root)
	uri := file("test.yaml")
	client.openDocument(uri, docText)

	client.sendRequest(2, "textDocument/codeLens", CodeLensParams{TextDocument: TextDocumentIdentifier{URI: uri}})
	var lenses []CodeLens
	if err := json.Unmarshal(client.readResponse(), &lenses); err != nil {
		t.Fatalf("unmarshal code lenses: %v", err)
	}
	if tc.Expect.CodeLenses == nil {
		return
	}
	got := []string{}
	for _, l := range lenses {
		if l.Command == nil {
			t.Fatalf("lens without a command: %+v", l)
		}
		if len(l.Command.Arguments) != 1 || l.Command.Arguments[0] != uri {
			t.Errorf("lens arguments = %v, want [%s]", l.Command.Arguments, uri)
		}
		got = append(got, fmt.Sprintf("%d:%s:%s", l.Range.Start.Line, l.Command.Title, l.Command.Command))
	}
	if !slices.Equal(got, *tc.Expect.CodeLenses) {
		t.Errorf("code lenses = %v, want %v", got, *tc.Expect.CodeLenses)
	}
}
