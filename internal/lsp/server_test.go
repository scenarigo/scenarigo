package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

// newRunningTestClient starts a server on in-process pipes and returns its
// client. The server stops when the pipes are closed at the end of the test.
func newRunningTestClient(t *testing.T) *testClient {
	t.Helper()
	srv, client := newTestClient(t)
	go func() { _ = srv.Run(context.Background()) }()
	return client
}
