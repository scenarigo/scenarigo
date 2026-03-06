package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
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
	}
	return srv, &testClient{t: t, inW: inW, outR: bufio.NewReader(outR)}
}

func (c *testClient) sendRequest(id int, method string, params string) {
	c.t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, params)
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	if _, err := io.WriteString(c.inW, msg); err != nil {
		c.t.Fatalf("write request: %v", err)
	}
}

func (c *testClient) sendNotification(method string, params string) {
	c.t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":%q,"params":%s}`, method, params)
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
	c.sendNotification("textDocument/didOpen", fmt.Sprintf(`{
		"textDocument": {
			"uri": %q,
			"languageId": "yaml",
			"version": 1,
			"text": %s
		}
	}`, uri, jsonString(text)))
	time.Sleep(10 * time.Millisecond)
	c.readMessage() // diagnostics
}

func (c *testClient) openDocumentAndGetDiagnostics(uri, text string) PublishDiagnosticsParams {
	c.t.Helper()
	c.sendNotification("textDocument/didOpen", fmt.Sprintf(`{
		"textDocument": {
			"uri": %q,
			"languageId": "yaml",
			"version": 1,
			"text": %s
		}
	}`, uri, jsonString(text)))
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
	c.sendRequest(id, "textDocument/completion", fmt.Sprintf(`{
		"textDocument": {"uri": %q},
		"position": {"line": %d, "character": %d}
	}`, uri, line, char))
	resp := c.readResponse()
	var list CompletionList
	if err := json.Unmarshal(resp, &list); err != nil {
		c.t.Fatalf("unmarshal completion: %v", err)
	}
	return list
}

func TestServer_Initialize(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	result := client.readResponse()

	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if initResult.Capabilities.CompletionProvider == nil {
		t.Fatal("expected completion provider")
	}
}

func TestServer_Completion_ScenarioKeys(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	scenarioText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    "
	client.openDocument("file:///tmp/test.yaml", scenarioText)

	list := client.complete(2, "file:///tmp/test.yaml", 5, 4)

	labels := labelSet(list.Items)
	for _, key := range []string{"request", "expect", "bind", "vars", "timeout"} {
		if !labels[key] {
			t.Errorf("expected %q in completions, got: %v", key, labelList(list.Items))
		}
	}
	for _, key := range []string{"title", "protocol"} {
		if labels[key] {
			t.Errorf("should not have %q (already exists)", key)
		}
	}
}

func TestServer_Completion_ValueEnum(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	scenarioText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: "
	client.openDocument("file:///tmp/test.yaml", scenarioText)

	list := client.complete(2, "file:///tmp/test.yaml", 4, 15)

	labels := labelSet(list.Items)
	if !labels["http"] || !labels["grpc"] {
		t.Errorf("expected http and grpc, got: %v", labelList(list.Items))
	}
}

func TestServer_Completion_ConfigKeys(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	configText := "schemaVersion: config/v1\n"
	client.openDocument("file:///tmp/scenarigo.yaml", configText)

	list := client.complete(2, "file:///tmp/scenarigo.yaml", 1, 0)

	labels := labelSet(list.Items)
	for _, key := range []string{"scenarios", "plugins", "protocols", "output", "input", "execution"} {
		if !labels[key] {
			t.Errorf("expected %q in config completions, got: %v", key, labelList(list.Items))
		}
	}
}

func TestServer_Completion_DynamicHTTPRequest(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	// Step with protocol: http, cursor inside request:
	scenarioText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    request:\n      "
	client.openDocument("file:///tmp/test.yaml", scenarioText)

	list := client.complete(2, "file:///tmp/test.yaml", 6, 6)

	labels := labelSet(list.Items)
	for _, key := range []string{"method", "url", "body", "header"} {
		if !labels[key] {
			t.Errorf("expected HTTP request field %q, got: %v", key, labelList(list.Items))
		}
	}
	// gRPC-only fields should not appear.
	for _, key := range []string{"service", "target", "metadata"} {
		if labels[key] {
			t.Errorf("should not have gRPC field %q in HTTP request", key)
		}
	}
}

func TestServer_Completion_DynamicGRPCRequest(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	scenarioText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: grpc\n    request:\n      "
	client.openDocument("file:///tmp/test.yaml", scenarioText)

	list := client.complete(2, "file:///tmp/test.yaml", 6, 6)

	labels := labelSet(list.Items)
	for _, key := range []string{"target", "service", "method", "message"} {
		if !labels[key] {
			t.Errorf("expected gRPC request field %q, got: %v", key, labelList(list.Items))
		}
	}
	// HTTP-only fields should not appear.
	if labels["url"] {
		t.Error("should not have HTTP field 'url' in gRPC request")
	}
}

func TestServer_Completion_DynamicHTTPExpect(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	scenarioText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    request:\n      method: GET\n      url: http://example.com\n    expect:\n      "
	client.openDocument("file:///tmp/test.yaml", scenarioText)

	list := client.complete(2, "file:///tmp/test.yaml", 9, 6)

	labels := labelSet(list.Items)
	for _, key := range []string{"code", "header", "body"} {
		if !labels[key] {
			t.Errorf("expected HTTP expect field %q, got: %v", key, labelList(list.Items))
		}
	}
	// gRPC-only expect fields should not appear.
	for _, key := range []string{"trailer", "status"} {
		if labels[key] {
			t.Errorf("should not have gRPC field %q in HTTP expect", key)
		}
	}
}

func TestServer_Definition_Include(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	// Create a temp file to link to.
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "included.yaml")
	os.WriteFile(targetFile, []byte("title: included\n"), 0o644)

	scenarioText := fmt.Sprintf("schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - include: %s\n", "included.yaml")
	docURI := "file://" + filepath.Join(tmpDir, "test.yaml")
	client.openDocument(docURI, scenarioText)

	// Request definition on the include value.
	client.sendRequest(2, "textDocument/definition", fmt.Sprintf(`{
		"textDocument": {"uri": %q},
		"position": {"line": 3, "character": 16}
	}`, docURI))
	resp := client.readResponse()

	var loc Location
	if err := json.Unmarshal(resp, &loc); err != nil {
		t.Fatalf("unmarshal location: %v", err)
	}
	if !strings.HasSuffix(loc.URI, "included.yaml") {
		t.Errorf("expected URI ending with included.yaml, got: %s", loc.URI)
	}
}

func TestServer_Diagnostics_UnknownKey(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	// Scenario with unknown key "unknownField".
	scenarioText := "schemaVersion: scenario/v1\ntitle: test\nunknownField: hello\nsteps:\n  - title: step1\n    protocol: http\n    badKey: value\n"
	diags := client.openDocumentAndGetDiagnostics("file:///tmp/test.yaml", scenarioText)

	// Should have diagnostics for unknown keys.
	found := make(map[string]bool)
	for _, d := range diags.Diagnostics {
		found[d.Message] = true
	}

	if !found[`unknown field "unknownField"`] {
		t.Errorf("expected diagnostic for unknownField, got: %v", diagMessages(diags.Diagnostics))
	}
	if !found[`unknown field "badKey"`] {
		t.Errorf("expected diagnostic for badKey, got: %v", diagMessages(diags.Diagnostics))
	}
}

func TestServer_Diagnostics_ValidDocument(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run()

	client.sendRequest(1, "initialize", `{"rootUri":"file:///tmp"}`)
	client.readResponse()

	// Valid scenario - should have no diagnostics.
	scenarioText := "schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    request:\n      method: GET\n      url: http://example.com\n"
	diags := client.openDocumentAndGetDiagnostics("file:///tmp/test.yaml", scenarioText)

	if len(diags.Diagnostics) > 0 {
		t.Errorf("expected no diagnostics for valid document, got: %v", diagMessages(diags.Diagnostics))
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

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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
