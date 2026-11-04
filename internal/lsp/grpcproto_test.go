package lsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
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
	if d := time.Since(start); d > time.Second {
		t.Errorf("diagnostics took %v", d)
	}
}
