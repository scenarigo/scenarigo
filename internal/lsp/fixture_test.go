package lsp

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// lspTestCase represents a single LSP test case loaded from a YAML fixture file.
type lspTestCase struct {
	Name      string            `yaml:"name"`
	Document  string            `yaml:"document"`
	Operation string            `yaml:"operation"` // see runSingleFixture for the supported operations
	Files     map[string]string `yaml:"files"`
	Expect    lspExpect         `yaml:"expect"`
}

type lspExpect struct {
	DiagnosticMessages *messageMatcher `yaml:"diagnosticMessages"`
	Diagnostics        []diagMatcher   `yaml:"diagnostics"`
	DiagnosticCount    *int            `yaml:"diagnosticCount"`
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

// parseCursorMarker finds and removes the $0 cursor marker from the document text.
func parseCursorMarker(doc string) string {
	before, after, ok := strings.Cut(doc, "$0")
	if !ok {
		return doc
	}

	text := before + after
	return text
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

	docText := parseCursorMarker(tc.Document)

	switch tc.Operation {
	case "diagnostics":
		runDiagnosticsFixture(t, tc, docText)
	default:
		t.Fatalf("unknown operation: %s", tc.Operation)
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
