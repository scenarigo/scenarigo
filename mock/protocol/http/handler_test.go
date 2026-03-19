package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"

	"github.com/scenarigo/scenarigo/logger"
	"github.com/scenarigo/scenarigo/mock/protocol"
)

func TestHandler(t *testing.T) {
	type expect struct {
		code   int
		header http.Header
		body   string
	}
	type step struct {
		request func() *http.Request
		expect  *expect
	}
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			filename string
			steps    []step
		}{
			"http": {
				filename: "testdata/http.yaml",
				steps: []step{
					{
						request: func() *http.Request {
							return httptest.NewRequest(http.MethodGet, "/", nil)
						},
						expect: &expect{
							code: 200,
							header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							body: `{"message": "hello"}`,
						},
					},
				},
			},
			"http with expect": {
				filename: "testdata/http-expect.yaml",
				steps: []step{
					{
						request: func() *http.Request {
							return httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"message":"hello"}`))
						},
						expect: &expect{
							code: 200,
							header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							body: `{"message": "hello"}`,
						},
					},
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				f, err := os.Open(test.filename)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				var mocks []protocol.Mock
				if err := yaml.NewDecoder(f).Decode(&mocks); err != nil {
					t.Fatal(err)
				}
				iter := protocol.NewMockIterator(mocks)
				h := NewHandler(iter, logger.NewNopLogger())
				for _, step := range test.steps {
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, step.request())
					if got, expect := rec.Code, step.expect.code; got != expect {
						t.Errorf("expect code %d but got %d", expect, got)
					}
					if diff := cmp.Diff(step.expect.header, rec.Header()); diff != "" {
						t.Errorf("body differs (-want +got):\n%s", diff)
					}
					if diff := cmp.Diff(step.expect.body, strings.TrimSuffix(rec.Body.String(), "\n")); diff != "" {
						t.Errorf("body differs (-want +got):\n%s", diff)
					}
				}
				if err := iter.Stop(); err != nil {
					t.Fatalf("failed to stop iterator: %s", err)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			filename string
			steps    []step
		}{
			"invalid protocol": {
				filename: "testdata/invalid-protocol.yaml",
				steps: []step{
					{
						request: func() *http.Request {
							return httptest.NewRequest(http.MethodGet, "/", nil)
						},
						expect: &expect{
							code: 500,
							header: http.Header{
								"Content-Type": []string{"text/plain; charset=utf-8"},
							},
							body: `received HTTP request but the mock protocol is "invalid"`,
						},
					},
				},
			},
			"over request": {
				filename: "testdata/http.yaml",
				steps: []step{
					{
						request: func() *http.Request {
							return httptest.NewRequest(http.MethodGet, "/", nil)
						},
						expect: &expect{
							code: 200,
							header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							body: `{"message": "hello"}`,
						},
					},
					{
						request: func() *http.Request {
							return httptest.NewRequest(http.MethodGet, "/", nil)
						},
						expect: &expect{
							code: 500,
							header: http.Header{
								"Content-Type": []string{"text/plain; charset=utf-8"},
							},
							body: "no mocks remain",
						},
					},
				},
			},
			"http invalid path": {
				filename: "testdata/http-expect.yaml",
				steps: []step{
					{
						request: func() *http.Request {
							return httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"message":"hello"}`))
						},
						expect: &expect{
							code: 500,
							header: http.Header{
								"Content-Type": []string{"text/plain; charset=utf-8"},
							},
							body: `assertion error: .path: expected "/echo" but got "/"`,
						},
					},
				},
			},
			"http invalid header": {
				filename: "testdata/http-expect.yaml",
				steps: []step{
					{
						request: func() *http.Request {
							r := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("hello"))
							r.Header.Add("Content-Type", "text/plain")
							return r
						},
						expect: &expect{
							code: 500,
							header: http.Header{
								"Content-Type": []string{"text/plain; charset=utf-8"},
							},
							body: `assertion error: .header.Content-Type: doesn't contain expected value: last error: expected "application/json" but got "text/plain"`,
						},
					},
				},
			},
			"http invalid body": {
				filename: "testdata/http-expect.yaml",
				steps: []step{
					{
						request: func() *http.Request {
							return httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"message":""}`))
						},
						expect: &expect{
							code: 500,
							header: http.Header{
								"Content-Type": []string{"text/plain; charset=utf-8"},
							},
							body: `assertion error: .body.message: expected not zero value`,
						},
					},
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				f, err := os.Open(test.filename)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				var mocks []protocol.Mock
				if err := yaml.NewDecoder(f).Decode(&mocks); err != nil {
					t.Fatal(err)
				}
				iter := protocol.NewMockIterator(mocks)
				h := NewHandler(iter, logger.NewNopLogger())
				for _, step := range test.steps {
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, step.request())
					if got, expect := rec.Code, step.expect.code; got != expect {
						t.Errorf("expect code %d but got %d", expect, got)
					}
					if diff := cmp.Diff(step.expect.header, rec.Header()); diff != "" {
						t.Errorf("body differs (-want +got):\n%s", diff)
					}
					if diff := cmp.Diff(step.expect.body, strings.TrimSuffix(rec.Body.String(), "\n")); diff != "" {
						t.Errorf("body differs (-want +got):\n%s", diff)
					}
				}
				if err := iter.Stop(); err != nil {
					t.Fatalf("failed to stop iterator: %s", err)
				}
			})
		}
	})
}

func TestExtract(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			resp *Response
		}{
			"invalid status code": {
				resp: &Response{
					Code: "OK",
				},
			},
			"invalid header key": {
				resp: &Response{
					Header: yaml.MapSlice{
						yaml.MapItem{
							Key: nil,
						},
					},
				},
			},
			"failed to marshal response body": {
				resp: &Response{
					Header: yaml.MapSlice{
						yaml.MapItem{
							Key:   "Content-Type",
							Value: "text/plain",
						},
					},
					Body: t,
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				if _, _, _, err := test.resp.extract(); err == nil {
					t.Fatalf("no error")
				}
			})
		}
	})
}
