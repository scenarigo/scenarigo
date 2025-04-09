package http

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"golang.org/x/text/encoding/japanese"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/internal/testutil"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/version"
	"github.com/zoncoen/query-go"
)

func TestRequestExtractor(t *testing.T) {
	req := &RequestExtractor{
		Method: http.MethodPost,
		URL:    "http://example.com",
		Header: http.Header{
			"Accept-Encoding": {"gzip"},
		},
		Body: map[string]string{"message": "hey"},
	}
	tests := map[string]struct {
		query       string
		expect      any
		expectError string
	}{
		"method": {
			query:  ".method",
			expect: req.Method,
		},
		"url": {
			query:  ".url",
			expect: req.URL,
		},
		"header": {
			query:  ".header.Accept-Encoding[0]",
			expect: "gzip",
		},
		"body": {
			query:  ".body.message",
			expect: "hey",
		},
		"body (backward compatibility)": {
			query:  ".message",
			expect: "hey",
		},
		"not found": {
			query:       ".body.aaa",
			expectError: `".body.aaa" not found`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			q, err := query.ParseString(
				test.query,
				queryutil.Options()...,
			)
			if err != nil {
				t.Fatal(err)
			}
			v, err := q.Extract(req)
			if test.expectError == "" && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if test.expectError != "" {
				if err == nil {
					t.Fatal("no error")
				} else if !strings.Contains(err.Error(), test.expectError) {
					t.Fatalf("expect %q but got %q", test.expectError, err)
				}
			}
			if got, expect := v, test.expect; got != expect {
				t.Fatalf("expect %v but got %v", expect, got)
			}
		})
	}
}

func TestResponseExtractor(t *testing.T) {
	resp := &ResponseExtractor{
		Status:     "200 OK",
		StatusCode: 200,
		Header: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: map[string]string{"message": "hey"},
	}
	tests := map[string]struct {
		query       string
		expect      any
		expectError string
	}{
		"status": {
			query:  ".status",
			expect: resp.Status,
		},
		"statusCode": {
			query:  ".statusCode",
			expect: resp.StatusCode,
		},
		"header": {
			query:  ".header.Content-Type[0]",
			expect: "application/json",
		},
		"body": {
			query:  ".body.message",
			expect: "hey",
		},
		"body (backward compatibility)": {
			query:  ".message",
			expect: "hey",
		},
		"not found": {
			query:       ".body.aaa",
			expectError: `".body.aaa" not found`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			q, err := query.ParseString(
				test.query,
				queryutil.Options()...,
			)
			if err != nil {
				t.Fatal(err)
			}
			v, err := q.Extract(resp)
			if test.expectError == "" && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if test.expectError != "" {
				if err == nil {
					t.Fatal("no error")
				} else if !strings.Contains(err.Error(), test.expectError) {
					t.Fatalf("expect %q but got %q", test.expectError, err)
				}
			}
			if got, expect := v, test.expect; got != expect {
				t.Fatalf("expect %v but got %v", expect, got)
			}
		})
	}
}

type transport struct {
	f func(*http.Request) (*http.Response, error)
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.f(req)
}

func roundTripper(f func(req *http.Request) (*http.Response, error)) http.RoundTripper {
	return &transport{f}
}

func TestRequest_Invoke(t *testing.T) {
	auth := "Bearer xxxxx"
	m := http.NewServeMux()
	m.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {})
	m.HandleFunc("/echo", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if req.Header.Get("Authorization") != auth {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		d := json.NewDecoder(req.Body)
		defer req.Body.Close()
		body := map[string]string{}
		if err := d.Decode(&body); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"message": "%s", "id": "%s"}`, body["message"], req.URL.Query().Get("id"))
	})
	m.HandleFunc("/echo/gzipped", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if req.Header.Get("Authorization") != auth {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if req.Header.Get("Accept-Encoding") != "gzip" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		d := json.NewDecoder(req.Body)
		defer req.Body.Close()
		body := map[string]string{}
		if err := d.Decode(&body); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		res := []byte(fmt.Sprintf(`{"message": "%s", "id": "%s"}`, body["message"], req.URL.Query().Get("id")))
		gz := new(bytes.Buffer)
		ww := gzip.NewWriter(gz)
		if _, err := ww.Write(res); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := ww.Close(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		if _, err := gz.WriteTo(w); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
	m.HandleFunc("/echo/shift_jis", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if req.Header.Get("Authorization") != auth {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		d := json.NewDecoder(req.Body)
		defer req.Body.Close()
		body := map[string]string{}
		if err := d.Decode(&body); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		b, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(fmt.Sprintf(`{"message": "%s", "id": "%s"}`, body["message"], req.URL.Query().Get("id"))))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=Shift_JIS")
		_, _ = w.Write(b)
	})
	srv := httptest.NewServer(m)
	defer srv.Close()

	tests := map[string]struct {
		vars        interface{}
		request     *Request
		response    response
		requestDump *RequestExtractor
	}{
		"default": {
			request: &Request{
				URL: srv.URL,
			},
			response: response{
				Status:     "200 OK",
				StatusCode: 200,
			},
			requestDump: &RequestExtractor{
				Method: http.MethodGet,
				URL:    srv.URL,
				Header: http.Header{
					"Accept-Encoding": {"gzip"},
					"User-Agent":      {fmt.Sprintf("scenarigo/%s", version.String())},
				},
			},
		},
		"POST": {
			request: &Request{
				Method: http.MethodPost,
				URL:    srv.URL + "/echo",
				Query:  url.Values{"id": []string{"123"}},
				Header: map[string][]string{"Authorization": {auth}},
				Body:   map[string]string{"message": "hey"},
			},
			response: response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       map[string]interface{}{"message": "hey", "id": "123"},
			},
			requestDump: &RequestExtractor{
				Method: http.MethodPost,
				URL:    srv.URL + "/echo?id=123",
				Header: http.Header{
					"Accept-Encoding": {"gzip"},
					"Authorization":   {auth},
					"User-Agent":      {fmt.Sprintf("scenarigo/%s", version.String())},
				},
				Body: map[string]string{"message": "hey"},
			},
		},
		"lower case method": {
			request: &Request{
				Method: "post",
				URL:    srv.URL + "/echo",
				Query:  url.Values{"id": []string{"123"}},
				Header: map[string][]string{"Authorization": {auth}},
				Body:   map[string]string{"message": "hey"},
			},
			response: response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       map[string]interface{}{"message": "hey", "id": "123"},
			},
			requestDump: &RequestExtractor{
				Method: http.MethodPost,
				URL:    srv.URL + "/echo?id=123",
				Header: http.Header{
					"Accept-Encoding": {"gzip"},
					"Authorization":   {auth},
					"User-Agent":      {fmt.Sprintf("scenarigo/%s", version.String())},
				},
				Body: map[string]string{"message": "hey"},
			},
		},
		"POST (gzipped)": {
			request: &Request{
				Method: http.MethodPost,
				URL:    srv.URL + "/echo/gzipped",
				Query:  url.Values{"id": []string{"123"}},
				Header: map[string][]string{
					"Authorization": {auth},
				},
				Body: map[string]string{"message": "hey"},
			},
			response: response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       map[string]interface{}{"message": "hey", "id": "123"},
			},
			requestDump: &RequestExtractor{
				Method: http.MethodPost,
				URL:    srv.URL + "/echo/gzipped?id=123",
				Header: http.Header{
					"Accept-Encoding": {"gzip"},
					"Authorization":   {auth},
					"User-Agent":      {fmt.Sprintf("scenarigo/%s", version.String())},
				},
				Body: map[string]string{"message": "hey"},
			},
		},
		"POST (Shift_JIS)": {
			request: &Request{
				Method: http.MethodPost,
				URL:    srv.URL + "/echo/shift_jis",
				Query:  url.Values{"id": []string{"123"}},
				Header: map[string][]string{
					"Authorization": {auth},
				},
				Body: map[string]string{"message": "hey"},
			},
			response: response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       map[string]interface{}{"message": "hey", "id": "123"},
			},
			requestDump: &RequestExtractor{
				Method: http.MethodPost,
				URL:    srv.URL + "/echo/shift_jis?id=123",
				Header: http.Header{
					"Accept-Encoding": {"gzip"},
					"Authorization":   {auth},
					"User-Agent":      {fmt.Sprintf("scenarigo/%s", version.String())},
				},
				Body: map[string]string{"message": "hey"},
			},
		},
		"with vars": {
			vars: map[string]string{
				"url":     srv.URL + "/echo",
				"auth":    auth,
				"message": "hey",
				"id":      "123",
			},
			request: &Request{
				Method: http.MethodPost,
				URL:    "{{vars.url}}",
				Query:  map[string]string{"id": "{{vars.id}}"},
				Header: map[string][]string{"Authorization": {"{{vars.auth}}"}},
				Body:   map[string]string{"message": "{{vars.message}}"},
			},
			response: response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       map[string]interface{}{"message": "hey", "id": "123"},
			},
			requestDump: &RequestExtractor{
				Method: http.MethodPost,
				URL:    srv.URL + "/echo?id=123",
				Header: http.Header{
					"Accept-Encoding": {"gzip"},
					"Authorization":   {auth},
					"User-Agent":      {fmt.Sprintf("scenarigo/%s", version.String())},
				},
				Body: map[string]string{"message": "hey"},
			},
		},
		"custom client": {
			vars: map[string]interface{}{
				"client": &http.Client{
					Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
						req.Header.Set("Authorization", auth)
						return http.DefaultTransport.RoundTrip(req)
					}),
				},
			},
			request: &Request{
				Client: "{{vars.client}}",
				Method: http.MethodPost,
				URL:    srv.URL + "/echo",
				Query:  url.Values{"id": []string{"123"}},
				Body:   map[string]string{"message": "hey"},
			},
			response: response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       map[string]interface{}{"message": "hey", "id": "123"},
			},
			requestDump: &RequestExtractor{
				Method: http.MethodPost,
				URL:    srv.URL + "/echo?id=123",
				Header: http.Header{
					"Authorization": {auth},
					"User-Agent":    {fmt.Sprintf("scenarigo/%s", version.String())},
				},
				Body: map[string]string{"message": "hey"},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.FromT(t)
			if test.vars != nil {
				ctx = ctx.WithVars(test.vars)
			}

			ctx, res, err := test.request.Invoke(ctx)
			if err != nil {
				t.Fatalf("failed to invoke: %s", err)
			}
			actualRes, ok := res.(response)
			if !ok {
				t.Fatalf("failed to convert from %T to response", res)
			}
			if diff := cmp.Diff(test.response.Body, actualRes.Body,
				cmp.AllowUnexported(
					response{},
				),
			); diff != "" {
				t.Fatalf("differs: (-want +got)\n%s", diff)
			}

			// ensure that ctx.WithRequest and ctx.WithResponse are called
			if diff := cmp.Diff(test.requestDump, ctx.Request()); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
			if diff := cmp.Diff((*ResponseExtractor)(&test.response), ctx.Response(), cmpopts.IgnoreFields(ResponseExtractor{}, "Header")); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestRequest_Invoke_Error(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("/unknown_charset", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Content-Type", "text/plain; charset=unknown")
	})
	m.HandleFunc("/invalid_content_type", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Content-Type", ";")
	})
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)

	tests := map[string]struct {
		vars    interface{}
		request *Request
		expect  string
	}{
		"URL is required": {
			request: &Request{},
			expect:  `failed to send request: Get "": unsupported protocol scheme ""`,
		},
		"failed to send request": {
			vars: map[string]interface{}{
				"client": &http.Client{
					Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
						return nil, errors.New("error occurred")
					}),
				},
			},
			request: &Request{
				Client: "{{vars.client}}",
				URL:    "http://localhost",
			},
			expect: `failed to send request: Get "http://localhost": error occurred`,
		},
		"failed to execute template": {
			request: &Request{
				URL: "{{vars.url}}",
			},
			expect: `.url: failed to get URL: failed to execute: {{vars.url}}: ".vars.url" not found`,
		},
		"failed to buildClient": {
			request: &Request{Client: "{{}}"},
			expect:  `.client: client must be "*http.Client" but got "string"`,
		},
		"failed to buildClient ( invalid template )": {
			request: &Request{Client: "{{invalid}}"},
			expect:  `.client: failed to get client: failed to execute: {{invalid}}: ".invalid" not found`,
		},
		"failed to parse Content-Type": {
			request: &Request{
				URL: fmt.Sprintf("%s/invalid_content_type", srv.URL),
			},
			expect: `failed to parse Content-Type response header ";": mime: no media type`,
		},
		"unknown caharset": {
			request: &Request{
				URL: fmt.Sprintf("%s/unknown_charset", srv.URL),
			},
			expect: `failed to decode response body: unknown cahrset "unknown"`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.FromT(t)
			if test.vars != nil {
				ctx = ctx.WithVars(test.vars)
			}
			_, _, err := test.request.Invoke(ctx)
			if err == nil {
				t.Fatal("no error")
			}
			if got := err.Error(); !strings.Contains(got, test.expect) {
				t.Errorf("%q doesn't contain %q", got, test.expect)
			}
		})
	}
}

func TestRequest_Invoke_Log(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		m := http.NewServeMux()
		m.HandleFunc("/echo", func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			d := json.NewDecoder(req.Body)
			defer req.Body.Close()
			body := map[string]string{}
			if err := d.Decode(&body); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"message": "%s"}`, body["message"])
		})
		srv := httptest.NewServer(m)
		defer srv.Close()

		var b bytes.Buffer
		reporter.Run(func(rptr reporter.Reporter) {
			rptr.Run("test.yaml", func(rptr reporter.Reporter) {
				ctx := context.New(rptr)
				req := &Request{
					Method: http.MethodPost,
					URL:    srv.URL + "/echo",
					Query:  url.Values{"query": []string{"hello"}},
					Body:   map[string]string{"message": "hey"},
				}
				if _, _, err := req.Invoke(ctx); err != nil {
					t.Fatalf("failed to invoke: %s", err)
				}
			})
		}, reporter.WithWriter(&b), reporter.WithVerboseLog())

		expect := strings.TrimPrefix(`
=== RUN   test.yaml
--- PASS: test.yaml (0.00s)
        request:
          method: POST
          url: http://127.0.0.1:12345/echo?query=hello
          header:
            User-Agent:
            - scenarigo/v1.0.0
          body:
            message: hey
        response:
          status: 200 OK
          statusCode: 200
          header:
            Content-Length:
            - "18"
            Content-Type:
            - application/json
            Date:
            - Mon, 01 Jan 0001 00:00:00 GMT
          body:
            message: hey
PASS
ok  	test.yaml	0.000s
`, "\n")
		if diff := cmp.Diff(expect, testutil.ReplaceOutput(b.String())); diff != "" {
			t.Errorf("differs (-want +got):\n%s", diff)
		}
	})
}

func TestRequest_buildRequest(t *testing.T) {
	tests := map[string]struct {
		req        *Request
		expectReq  func(*testing.T) *http.Request
		expectBody interface{}
	}{
		"empty request": {
			req: &Request{},
			expectReq: func(t *testing.T) *http.Request {
				t.Helper()
				req, err := http.NewRequest(http.MethodGet, "", nil)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				req.Header.Set("User-Agent", defaultUserAgent)
				return req
			},
		},
		"with User-Agent": {
			req: &Request{
				Header: map[string]string{"User-Agent": "custom/0.0.1"},
			},
			expectReq: func(t *testing.T) *http.Request {
				t.Helper()
				req, err := http.NewRequest(http.MethodGet, "", nil)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				req.Header.Set("User-Agent", "custom/0.0.1")
				return req
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.FromT(t)
			req, body, err := test.req.buildRequest(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(test.expectReq(t), req, cmp.AllowUnexported(http.Request{}), cmp.FilterPath(func(path cmp.Path) bool {
				return path.String() == "ctx"
			}, cmp.Ignore())); diff != "" {
				t.Errorf("request differs (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.expectBody, body); diff != "" {
				t.Errorf("body differs (-want +got):\n%s", diff)
			}
		})
	}
}
