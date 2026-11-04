package lsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCompileProtosRetriesFailures(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "svc.proto"), "syntax = \"proto3\";\npackage p;\nimport \"common.proto\";\nservice S { rpc M(common.Req) returns (common.Req); }\n")
	common := filepath.Join(dir, "common.proto")
	writeFile(t, common, "syntax = \"proto3\";\npackage common;\nmessage Req {\n")

	s := NewServer()
	s.logger = log.New(io.Discard, "", 0)
	imports, files := []string{dir}, []string{"svc.proto"}
	if fds := s.compileProtos(imports, files); fds != nil {
		t.Fatal("a broken import compiled")
	}

	// The fixed import is not one of the listed files: the failure stays
	// cached until it is tried again.
	writeFile(t, common, "syntax = \"proto3\";\npackage common;\nmessage Req { string a = 1; }\n")
	if fds := s.compileProtos(imports, files); fds != nil {
		t.Fatal("a failed compilation was tried again at once")
	}
	s.protoCache[protoCacheKey(imports, files)].retryAt = time.Now().Add(-time.Second)
	if fds := s.compileProtos(imports, files); fds == nil {
		t.Fatal("a failed compilation was not tried again")
	}
}

func TestProtoRetryDelay(t *testing.T) {
	timeout := protoRetryDelay(context.DeadlineExceeded)
	failure := protoRetryDelay(errors.New("syntax error"))
	if timeout <= failure {
		t.Errorf("a timeout is tried again after %v, a failure after %v", timeout, failure)
	}
}

func TestSiblingKeysOutOfRange(t *testing.T) {
	lines := []string{"message:", "  a: 1"}
	if keys := siblingKeys(lines, 5, keySlot{}); keys != nil {
		t.Errorf("keys = %v", keys)
	}
}

// TestProtoAliasBomb opens a small file whose aliases expand to a million
// import paths: the server must refuse to expand it rather than block.
func TestProtoAliasBomb(t *testing.T) {
	var b strings.Builder
	b.WriteString("schemaVersion: scenario/v1\nvars:\n  l0: &l0 x.proto\n")
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&b, "  l%d: &l%d [%s]\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*l%d, ", i-1), 10), ", "))
	}
	b.WriteString("steps:\n  - protocol: grpc\n    request:\n      options:\n        proto:\n          imports: *l6\n          files: [svc.proto]\n      service: t.S\n")
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)
	client.initialize(1, root)
	start := time.Now()
	client.openDocumentAndGetDiagnostics(file("test.yaml"), b.String())
	if d := time.Since(start); d > time.Second*raceSlowdown {
		t.Errorf("diagnostics took %v", d)
	}
}

// protoSession opens a scenario in a workspace with a proto file and
// returns its diagnostics and how long they took.
func protoSession(t *testing.T, text string) ([]string, time.Duration) {
	t.Helper()
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)
	writeFile(t, filepath.Join(uriToPath(root), "proto", "a.proto"), "syntax = \"proto3\";\npackage a;\nservice A { rpc M(R) returns (R); }\nmessage R { string x = 1; }\n")
	writeFile(t, filepath.Join(uriToPath(root), "proto", "b.proto"), "syntax = \"proto3\";\npackage b;\nservice B { rpc M(R) returns (R); }\nmessage R { string y = 1; }\n")
	client.initialize(1, root)
	start := time.Now()
	diags := client.openDocumentAndGetDiagnostics(file("test.yaml"), text)
	return diagMessages(diags.Diagnostics), time.Since(start)
}

// TestProtoAliasBombWithRedefinedAnchors redefines the anchors of an alias
// bomb after their use: an alias resolves to the anchor before it.
func TestProtoAliasBombWithRedefinedAnchors(t *testing.T) {
	var b strings.Builder
	b.WriteString("schemaVersion: scenario/v1\nvars:\n  l0: &l0 x.proto\n")
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&b, "  l%d: &l%d [%s]\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*l%d, ", i-1), 10), ", "))
	}
	b.WriteString("steps:\n  - protocol: grpc\n    request:\n      options:\n        proto:\n          imports: *l6\n          files: [a.proto]\n      service: a.A\nanchors:\n")
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&b, "  r%d: &l%d x\n", i, i)
	}
	if _, d := protoSession(t, b.String()); d > time.Second*raceSlowdown {
		t.Errorf("diagnostics took %v", d)
	}
}

// TestProtoLargeDocument checks the last step of a scenario with 2,000
// gRPC steps.
func TestProtoLargeDocument(t *testing.T) {
	var b strings.Builder
	b.WriteString("schemaVersion: scenario/v1\nsteps:\n")
	for i := range 2000 {
		fmt.Fprintf(&b, "  - title: step %d\n    protocol: grpc\n    request:\n      options: {proto: {imports: [proto], files: [a.proto]}}\n      service: a.A\n      method: M\n      metadata: {k: v}\n      message: {x: a, bogus%d: 1}\n    expect:\n      code: OK\n      message: {x: a}\n", i, i)
	}
	messages, _ := protoSession(t, b.String())
	if !slices.Contains(messages, `unknown field "bogus1999" in a.R`) {
		t.Errorf("no warning on the last step (%d diagnostics)", len(messages))
	}
}

// TestProtoManyDocuments times the proto diagnostics of a file with 2,000
// documents of one step each: steps are found without scanning every
// document for each of them.
func TestProtoManyDocuments(t *testing.T) {
	var b strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&b, "---\nschemaVersion: scenario/v1\nsteps:\n  - protocol: grpc\n    request:\n      options: {proto: {imports: [proto], files: [a.proto]}}\n      service: a.A\n      method: M\n      message: {bogus%d: 1}\n", i)
	}
	root, file := newWorkspace(t)
	writeFile(t, filepath.Join(uriToPath(root), "proto", "a.proto"), "syntax = \"proto3\";\npackage a;\nservice A { rpc M(R) returns (R); }\nmessage R { string x = 1; }\n")
	s := NewServer()
	s.logger = log.New(io.Discard, "", 0)
	s.rootURI = root
	s.docs.Open(file("test.yaml"), 1, b.String())
	doc := s.docs.Get(file("test.yaml"))
	s.protoDiagnostics(doc) // compiles the proto file
	start := time.Now()
	messages := diagMessages(s.protoDiagnostics(doc))
	if d := time.Since(start); d > 500*time.Millisecond*raceSlowdown {
		t.Errorf("proto diagnostics took %v", d)
	}
	if !slices.Contains(messages, `unknown field "bogus1999" in a.R`) {
		t.Errorf("no warning on the last document (%d diagnostics)", len(messages))
	}
}

// TestProtoAliases checks aliases as scenarigo resolves them: to the anchor
// before them, also in an earlier document, and an alias being typed
// elsewhere does not matter.
func TestProtoAliases(t *testing.T) {
	text := `schemaVersion: scenario/v1
anchors:
  opts: &opts {proto: {imports: [proto], files: [a.proto]}}
  svc: &svc a.A
  host: &host localhost:8080
steps:
  - protocol: grpc
    request: {target: *host, options: *opts, service: *svc, method: M, message: {bogus1: 1}}
  - protocol: http
    request: {url: *bas}
---
schemaVersion: scenario/v1
anchors:
  opts: &opts {proto: {imports: [proto], files: [b.proto]}}
  svc: &svc b.B
steps:
  - protocol: grpc
    request: {target: *host, options: *opts, service: *svc, method: M, message: {bogus2: 1}}
`
	messages, _ := protoSession(t, text)
	for _, want := range []string{`unknown field "bogus1" in a.R`, `unknown field "bogus2" in b.R`} {
		if !slices.Contains(messages, want) {
			t.Errorf("no %s in %v", want, messages)
		}
	}
	for _, m := range messages {
		if strings.Contains(m, "unknown gRPC service") {
			t.Errorf("unexpected %s", m)
		}
	}
}

// TestProtoNullOptions: null options are no options, and those of
// scenarigo.yaml apply.
func TestProtoNullOptions(t *testing.T) {
	client := newRunningTestClient(t)
	root, file := newWorkspace(t)
	dir := uriToPath(root)
	writeFile(t, filepath.Join(dir, "proto", "a.proto"), "syntax = \"proto3\";\npackage a;\nservice A { rpc M(R) returns (R); }\nmessage R { string x = 1; }\n")
	writeFile(t, filepath.Join(dir, "scenarigo.yaml"), "schemaVersion: config/v1\nprotocols:\n  grpc:\n    request:\n      proto: {imports: [proto], files: [a.proto]}\n")
	client.initialize(1, root)
	diags := client.openDocumentAndGetDiagnostics(file("test.yaml"), "schemaVersion: scenario/v1\nsteps:\n  - protocol: grpc\n    request:\n      options:\n        # proto: {files: [b.proto]}\n      service: a.A\n      method: M\n      message: {bogus: 1}\n")
	if messages := diagMessages(diags.Diagnostics); !slices.Contains(messages, `unknown field "bogus" in a.R`) {
		t.Errorf("no warning in %v", messages)
	}
}

// TestProtoMergeBombs times merge keys that multiply: nested merges, and a
// large merged mapping referenced many times.
func TestProtoMergeBombs(t *testing.T) {
	nested := func() string {
		var b strings.Builder
		b.WriteString("schemaVersion: scenario/v1\nvars:\n  m0: &m0 {k: v}\n")
		for i := 1; i <= 8; i++ {
			fmt.Fprintf(&b, "  m%d: &m%d {<<: [%s]}\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*m%d, ", i-1), 10), ", "))
		}
		b.WriteString("  all: {<<: *m8}\nsteps:\n  - protocol: grpc\n    request:\n      options: {<<: *m8, proto: {imports: [proto], files: [a.proto]}}\n      service: a.A\n")
		return b.String()
	}()
	repeated := func() string {
		var b strings.Builder
		b.WriteString("schemaVersion: scenario/v1\nvars:\n  m0: &m0 {k: v}\n")
		for i := 1; i <= 4; i++ {
			fmt.Fprintf(&b, "  m%d: &m%d {<<: [%s]}\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*m%d, ", i-1), 10), ", "))
		}
		b.WriteString("  s0: &s0 [*m4]\n")
		for i := 1; i <= 4; i++ {
			fmt.Fprintf(&b, "  s%d: &s%d [%s]\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*s%d, ", i-1), 10), ", "))
		}
		b.WriteString("steps:\n  - protocol: grpc\n    request:\n      options: {auth: {tls: *s4}, proto: {imports: [proto], files: [a.proto]}}\n      service: a.A\n")
		return b.String()
	}()
	for name, text := range map[string]string{"nested": nested, "repeated": repeated} {
		if _, d := protoSession(t, text); d > 2*time.Second*raceSlowdown {
			t.Errorf("%s: diagnostics took %v", name, d)
		}
	}
}

// TestProtoAnchorsInTheirDocument: an alias takes the last anchor of its
// name in its own document, even one after it, as scenarigo's loader does.
func TestProtoAnchorsInTheirDocument(t *testing.T) {
	text := `schemaVersion: scenario/v1
steps:
  - protocol: grpc
    request: &req {options: {proto: {imports: [proto], files: [a.proto]}}, service: a.A, method: M, message: {x: a}}
  - protocol: grpc
    request: {<<: *req, message: {x: again}}
  - protocol: grpc
    request: &req {options: {proto: {imports: [proto], files: [b.proto]}}, service: b.B, method: M, message: {y: b}}
`
	messages, _ := protoSession(t, text)
	if !slices.Contains(messages, `unknown field "x" in b.R`) {
		t.Errorf("the merged request of step 2 is not the last one of the document: %v", messages)
	}
}
