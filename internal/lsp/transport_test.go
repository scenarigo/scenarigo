package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestReadMessage(t *testing.T) {
	frame := func(body string) string {
		return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr error
		errText string
	}{
		{
			name:  "single message",
			input: frame(`{"id":1}`),
			want:  []string{`{"id":1}`},
		},
		{
			name:  "two messages in one buffer",
			input: frame(`{"id":1}`) + frame(`{"id":2}`),
			want:  []string{`{"id":1}`, `{"id":2}`},
		},
		{
			name:  "header name is case-insensitive and Content-Type is ignored",
			input: "content-length: 8\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n{\"id\":1}",
			want:  []string{`{"id":1}`},
		},
		{
			name:  "non-ASCII body is measured in bytes",
			input: frame(`{"title":"日本語"}`),
			want:  []string{`{"title":"日本語"}`},
		},
		{
			name:    "missing Content-Length",
			input:   "Content-Type: application/json\r\n\r\n{}",
			errText: "missing Content-Length",
		},
		{
			name:    "negative Content-Length",
			input:   "Content-Length: -1\r\n\r\n",
			errText: "invalid Content-Length",
		},
		{
			name:    "non-numeric Content-Length",
			input:   "Content-Length: abc\r\n\r\n",
			errText: "invalid Content-Length",
		},
		{
			name:    "header line without a colon",
			input:   "garbage\r\n\r\n",
			errText: "invalid header line",
		},
		{
			name:    "truncated body",
			input:   "Content-Length: 10\r\n\r\n{}",
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name:    "stream ends inside the headers",
			input:   "Content-Length: 2\r\n",
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name:    "stream ends between messages",
			input:   "",
			wantErr: io.EOF,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(tt.input))
			for _, want := range tt.want {
				got, err := readMessage(r)
				if err != nil {
					t.Fatalf("readMessage: %v", err)
				}
				if string(got) != want {
					t.Errorf("got %q, want %q", got, want)
				}
			}
			_, err := readMessage(r)
			switch {
			case tt.errText != "":
				if err == nil || !strings.Contains(err.Error(), tt.errText) {
					t.Errorf("got error %v, want one containing %q", err, tt.errText)
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
			default:
				if !errors.Is(err, io.EOF) {
					t.Errorf("got error %v after the last message, want io.EOF", err)
				}
			}
		})
	}
}

// sendRaw writes an already-encoded JSON-RPC body with a valid frame.
func (c *testClient) sendRaw(body string) {
	c.t.Helper()
	if _, err := io.WriteString(c.inW, fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)); err != nil {
		c.t.Fatalf("write raw message: %v", err)
	}
}

// readError reads the next message and returns it as an error response.
func (c *testClient) readError() errorResponse {
	c.t.Helper()
	var resp errorResponse
	if err := json.Unmarshal(c.readMessage(), &resp); err != nil {
		c.t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error == nil {
		c.t.Fatal("expected an error response")
	}
	return resp
}

func TestServer_ProtocolErrors(t *testing.T) {
	srv, client := newTestClient(t)
	go srv.Run(context.Background())

	t.Run("parse error", func(t *testing.T) {
		client.sendRaw(`{"jsonrpc":"2.0","id":1,`)
		resp := client.readError()
		if resp.Error.Code != codeParseError {
			t.Errorf("code = %d, want %d", resp.Error.Code, codeParseError)
		}
		if resp.ID != nil && string(*resp.ID) != "null" {
			t.Errorf("id = %s, want null", *resp.ID)
		}
	})

	t.Run("method not found", func(t *testing.T) {
		client.sendRaw(`{"jsonrpc":"2.0","id":2,"method":"textDocument/rename","params":{}}`)
		resp := client.readError()
		if resp.Error.Code != codeMethodNotFound {
			t.Errorf("code = %d, want %d", resp.Error.Code, codeMethodNotFound)
		}
		if string(*resp.ID) != "2" {
			t.Errorf("id = %s, want 2", *resp.ID)
		}
	})

	t.Run("invalid params", func(t *testing.T) {
		client.sendRaw(`{"jsonrpc":"2.0","id":3,"method":"textDocument/completion","params":"oops"}`)
		resp := client.readError()
		if resp.Error.Code != codeInvalidParams {
			t.Errorf("code = %d, want %d", resp.Error.Code, codeInvalidParams)
		}
	})

	t.Run("unknown notification is ignored", func(t *testing.T) {
		client.sendRaw(`{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":1}}`)
		client.sendRequest(4, "shutdown", nil)
		raw := client.readMessage()
		if !strings.Contains(string(raw), `"id":4`) {
			t.Errorf("expected the shutdown response next, got %s", raw)
		}
		if !strings.Contains(string(raw), `"result":null`) {
			t.Errorf("expected a null result, got %s", raw)
		}
	})
}
