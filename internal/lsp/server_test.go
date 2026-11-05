package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type testClient struct {
	t    *testing.T
	inW  io.Writer
	outR *bufio.Reader
}

func newTestClient(t *testing.T) (*Server, *testClient) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := &Server{
		reader: inR,
		writer: outW,
		logger: log.New(io.Discard, "", 0),
		docs:   newDocumentStore(),
	}
	return srv, &testClient{t: t, inW: inW, outR: bufio.NewReader(outR)}
}

func (c *testClient) sendRequest(id int, method string, params any) {
	c.t.Helper()
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		c.t.Fatalf("marshal params: %v", err)
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, paramsJSON)
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	if _, err := io.WriteString(c.inW, msg); err != nil {
		c.t.Fatalf("write request: %v", err)
	}
}

func (c *testClient) sendNotification(method string, params any) {
	c.t.Helper()
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		c.t.Fatalf("marshal params: %v", err)
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":%q,"params":%s}`, method, paramsJSON)
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	if _, err := io.WriteString(c.inW, msg); err != nil {
		c.t.Fatalf("write notification: %v", err)
	}
}

func (c *testClient) readMessage() json.RawMessage {
	c.t.Helper()
	contentLength := -1
	for {
		line, err := c.outR.ReadString('\n')
		if err != nil {
			c.t.Fatalf("read header: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if after, ok := strings.CutPrefix(line, "Content-Length: "); ok {
			n, err := strconv.Atoi(after)
			if err != nil {
				c.t.Fatalf("parse Content-Length: %v", err)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		c.t.Fatal("no Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.outR, body); err != nil {
		c.t.Fatalf("read body: %v", err)
	}
	return body
}

func (c *testClient) readResponse() json.RawMessage {
	c.t.Helper()
	raw := c.readMessage()
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		c.t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		c.t.Fatalf("response error: %s", resp.Error.Message)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		c.t.Fatalf("marshal result: %v", err)
	}
	return b
}

func (c *testClient) openDocument(uri, text string) {
	c.t.Helper()
	c.sendNotification("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: "yaml",
			Version:    1,
			Text:       text,
		},
	})
	c.readMessage() // diagnostics
}

func (c *testClient) openDocumentAndGetDiagnostics(uri, text string) PublishDiagnosticsParams {
	c.t.Helper()
	c.sendNotification("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: "yaml",
			Version:    1,
			Text:       text,
		},
	})
	raw := c.readMessage()
	var notif Notification
	if err := json.Unmarshal(raw, &notif); err != nil {
		c.t.Fatalf("unmarshal notification: %v", err)
	}
	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notif.Params, &params); err != nil {
		c.t.Fatalf("unmarshal diagnostics: %v", err)
	}
	return params
}

func (c *testClient) complete(id int, uri string, line, char int) CompletionList {
	c.t.Helper()
	return c.completeWith(id, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: char},
	})
}

func (c *testClient) completeWith(id int, params CompletionParams) CompletionList {
	c.t.Helper()
	c.sendRequest(id, "textDocument/completion", params)
	resp := c.readResponse()
	var list CompletionList
	if err := json.Unmarshal(resp, &list); err != nil {
		c.t.Fatalf("unmarshal completion: %v", err)
	}
	return list
}

// --- typed helper methods ---

func (c *testClient) initialize(id int, rootURI string) InitializeResult {
	c.t.Helper()
	return c.initializeWith(id, InitializeParams{RootURI: rootURI})
}

func (c *testClient) initializeWith(id int, params InitializeParams) InitializeResult {
	c.t.Helper()
	c.sendRequest(id, "initialize", params)
	resp := c.readResponse()
	var result InitializeResult
	if err := json.Unmarshal(resp, &result); err != nil {
		c.t.Fatalf("unmarshal initialize result: %v", err)
	}
	return result
}

func TestServer_Initialize(t *testing.T) {
	client := newRunningTestClient(t)

	initResult := client.initialize(1, "file:///tmp")
	if initResult.Capabilities.CompletionProvider == nil {
		t.Fatal("expected completion provider")
	}
	// "{" and "." let clients that only complete on trigger characters offer
	// candidates while {{vars. is typed.
	for _, c := range []string{".", "{"} {
		if !slices.Contains(initResult.Capabilities.CompletionProvider.TriggerCharacters, c) {
			t.Errorf("trigger characters %q do not include %q", initResult.Capabilities.CompletionProvider.TriggerCharacters, c)
		}
	}
}

func TestServer_Fixtures(t *testing.T) {
	fixtureFiles, err := filepath.Glob("testdata/*/*.yaml")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(fixtureFiles) == 0 {
		t.Fatal("no fixture files found")
	}
	for _, f := range fixtureFiles {
		t.Run(filepath.Base(filepath.Dir(f))+"/"+strings.TrimSuffix(filepath.Base(f), ".yaml"), func(t *testing.T) {
			runLSPTestFixtures(t, f)
		})
	}
}

// --- helpers ---

func diagMessages(diags []Diagnostic) []string {
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.Message)
	}
	return msgs
}

func labelSet(items []CompletionItem) map[string]bool {
	m := make(map[string]bool)
	for _, item := range items {
		m[item.Label] = true
	}
	return m
}

func labelList(items []CompletionItem) []string {
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	return labels
}

// newWorkspace creates a temporary workspace root for a test and returns its
// URI together with a helper that builds URIs of files below it, so that
// tests never depend on files that happen to exist under /tmp.
func newWorkspace(t *testing.T) (string, func(string) string) {
	t.Helper()
	dir := t.TempDir()
	return pathToURI(dir), func(name string) string { return pathToURI(filepath.Join(dir, name)) }
}

// newRunningTestClient starts a server on in-process pipes and returns its
// client. The server stops when the pipes are closed at the end of the test.
func newRunningTestClient(t *testing.T) *testClient {
	t.Helper()
	srv, client := newTestClient(t)
	go func() { _ = srv.Run(context.Background()) }()
	return client
}
