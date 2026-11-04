package lsp

import (
	"context"
	"errors"
	"io"
	"log"
	"path/filepath"
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
