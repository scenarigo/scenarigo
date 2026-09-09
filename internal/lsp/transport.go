package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Run starts the LSP server main loop.
// It blocks until the context is canceled or the input stream is closed.
// An "exit" notification terminates the process instead of returning.
func (s *Server) Run(ctx context.Context) error {
	type readResult struct {
		body []byte
		err  error
	}
	ch := make(chan readResult, 1)

	go func() {
		reader := bufio.NewReader(s.reader)
		for {
			body, err := readMessage(reader)
			ch <- readResult{body, err}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-ch:
			if result.err != nil {
				if errors.Is(result.err, io.EOF) {
					return nil
				}
				return result.err
			}

			var req Request
			if err := json.Unmarshal(result.body, &req); err != nil {
				s.sendResponse(nil, nil, &ResponseError{
					Code:    codeParseError,
					Message: "parse error: " + err.Error(),
				})
				continue
			}

			s.handleMessage(&req)
		}
	}
}

// readMessage reads a single message of the LSP base protocol: header lines
// terminated by CRLF, an empty line, and a body of Content-Length bytes.
// It returns io.EOF when the stream ends between messages.
func readMessage(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && (line != "" || contentLength >= 0) {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid header line %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(key), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid Content-Length %q", strings.TrimSpace(value))
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, errors.New("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return body, nil
}

func (s *Server) sendResponse(id *json.RawMessage, result any, respErr *ResponseError) {
	if respErr != nil {
		s.writeMessage(errorResponse{JSONRPC: "2.0", ID: id, Error: respErr})
		return
	}
	s.writeMessage(Response{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) sendNotification(method string, params any) {
	p, err := json.Marshal(params)
	if err != nil {
		s.logger.Printf("marshal %s params: %v", method, err)
		return
	}
	s.writeMessage(Notification{JSONRPC: "2.0", Method: method, Params: p})
}

// writeMessage frames and writes one message. Requests are handled one at a
// time today, but the mutex keeps the framing intact if that ever changes.
func (s *Server) writeMessage(msg any) {
	body, err := json.Marshal(msg)
	if err != nil {
		s.logger.Printf("marshal error: %v", err)
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		s.logger.Printf("write header error: %v", err)
		return
	}
	if _, err := s.writer.Write(body); err != nil {
		s.logger.Printf("write body error: %v", err)
	}
}

// invalidParams builds the error for a request whose params do not unmarshal.
func invalidParams(method string, err error) *ResponseError {
	return &ResponseError{
		Code:    codeInvalidParams,
		Message: fmt.Sprintf("invalid %s params: %v", method, err),
	}
}
