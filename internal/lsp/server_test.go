package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
		config: serverConfig{Formatting: true},
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
		if strings.HasPrefix(line, "Content-Length: ") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
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
	b, _ := json.Marshal(resp.Result)
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
	time.Sleep(10 * time.Millisecond)
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
	time.Sleep(10 * time.Millisecond)
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
	c.sendRequest(id, "textDocument/completion", CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: char},
	})
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
	c.sendRequest(id, "initialize", InitializeParams{RootURI: rootURI})
	resp := c.readResponse()
	var result InitializeResult
	if err := json.Unmarshal(resp, &result); err != nil {
		c.t.Fatalf("unmarshal initialize result: %v", err)
	}
	return result
}

func (c *testClient) changeDocument(uri string, version int, newText string) {
	c.t.Helper()
	c.sendNotification("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: TextDocumentIdentifier{URI: uri},
			Version:                version,
		},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: newText}},
	})
	time.Sleep(10 * time.Millisecond)
	c.readMessage() // diagnostics after change
}

func (c *testClient) closeDocument(uri string) {
	c.t.Helper()
	c.sendNotification("textDocument/didClose", DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	time.Sleep(10 * time.Millisecond)
	c.readMessage() // clear diagnostics notification
}

func (c *testClient) hover(id int, uri string, line, char int) json.RawMessage {
	c.t.Helper()
	c.sendRequest(id, "textDocument/hover", HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: char},
	})
	return c.readResponse()
}

func (c *testClient) definition(id int, uri string, line, char int) json.RawMessage {
	c.t.Helper()
	c.sendRequest(id, "textDocument/definition", DefinitionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: char},
	})
	return c.readResponse()
}

func (c *testClient) references(id int, uri string, line, char int) json.RawMessage {
	c.t.Helper()
	c.sendRequest(id, "textDocument/references", ReferenceParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: char},
		Context:      ReferenceContext{IncludeDeclaration: true},
	})
	return c.readResponse()
}

func (c *testClient) documentSymbol(id int, uri string) json.RawMessage {
	c.t.Helper()
	c.sendRequest(id, "textDocument/documentSymbol", DocumentSymbolParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	return c.readResponse()
}

func (c *testClient) codeAction(id int, uri string, r Range, diags []Diagnostic) json.RawMessage {
	c.t.Helper()
	c.sendRequest(id, "textDocument/codeAction", CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range:        r,
		Context:      CodeActionContext{Diagnostics: diags},
	})
	return c.readResponse()
}

func (c *testClient) formatting(id int, uri string) json.RawMessage {
	c.t.Helper()
	c.sendRequest(id, "textDocument/formatting", DocumentFormattingParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Options:      FormattingOptions{TabSize: 2, InsertSpaces: true},
	})
	return c.readResponse()
}

func (c *testClient) signatureHelp(id int, uri string, line, char int) json.RawMessage {
	c.t.Helper()
	c.sendRequest(id, "textDocument/signatureHelp", SignatureHelpParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: char},
	})
	return c.readResponse()
}

func (c *testClient) shutdown(id int) {
	c.t.Helper()
	c.sendRequest(id, "shutdown", struct{}{})
	c.readResponse()
}

func TestServer_Initialize(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	initResult := client.initialize(1, "file:///tmp")
	if initResult.Capabilities.CompletionProvider == nil {
		t.Fatal("expected completion provider")
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
