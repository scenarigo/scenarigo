package http

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/internal/testutil"
	"github.com/zoncoen/scenarigo/reporter"
)

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
	t.Run("success", func(t *testing.T) {
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
			_, _ = w.Write([]byte(fmt.Sprintf(`{"message": "%s", "id": "%s"}`, body["message"], req.URL.Query().Get("id"))))
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
		srv := httptest.NewServer(m)
		defer srv.Close()

		tests := map[string]struct {
			vars     interface{}
			request  *Request
			response response
		}{
			"default": {
				request: &Request{
					URL: srv.URL,
				},
				response: response{
					status: "200 OK",
				},
			},
			"Post": {
				request: &Request{
					Method: http.MethodPost,
					URL:    srv.URL + "/echo",
					Query:  url.Values{"id": []string{"123"}},
					Header: map[string][]string{"Authorization": {auth}},
					Body:   map[string]string{"message": "hey"},
				},
				response: response{
					status: "200 OK",
					Body:   map[string]interface{}{"message": "hey", "id": "123"},
				},
			},
			"Post (gzipped)": {
				request: &Request{
					Method: http.MethodPost,
					URL:    srv.URL + "/echo/gzipped",
					Query:  url.Values{"id": []string{"123"}},
					Header: map[string][]string{
						"Authorization":   {auth},
						"Accept-Encoding": {"gzip"},
					},
					Body: map[string]string{"message": "hey"},
				},
				response: response{
					status: "200 OK",
					Body:   map[string]interface{}{"message": "hey", "id": "123"},
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
					status: "200 OK",
					Body:   map[string]interface{}{"message": "hey", "id": "123"},
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
					status: "200 OK",
					Body:   map[string]interface{}{"message": "hey", "id": "123"},
				},
			},
		}
		for name, test := range tests {
			test := test
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
				if diff := cmp.Diff(test.request.Body, ctx.Request()); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
				if diff := cmp.Diff(test.response.Body, ctx.Response()); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			vars    interface{}
			request *Request
		}{
			"URL is required": {
				request: &Request{},
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
			},
			"failed to execute template": {
				request: &Request{
					URL: "{{vars.url}}",
				},
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				ctx := context.FromT(t)
				if test.vars != nil {
					ctx = ctx.WithVars(test.vars)
				}
				_, _, err := test.request.Invoke(ctx)
				if err == nil {
					t.Fatal("no error")
				}
			})
		}
	})
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
			_, _ = w.Write([]byte(fmt.Sprintf(`{"message": "%s"}`, body["message"])))
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

		expect := fmt.Sprintf(`
=== RUN   test.yaml
--- PASS: test.yaml (0.00s)
    request:
        method: POST
        url: %s/echo?query=hello
        header:
          User-Agent:
          - %s
        body:
          message: hey
    response:
        header:
          Content-Length:
          - "18"
          Content-Type:
          - application/json
        body:
          message: hey
PASS
ok  	test.yaml	0.000s
`, srv.URL, defaultUserAgent)
		if diff := cmp.Diff(expect, "\n"+testutil.ResetDuration(removeDateHeader(b.String()))); diff != "" {
			t.Errorf("differs (-want +got):\n%s", diff)
		}
	})
}

var dateHeaderPattern = regexp.MustCompile(`\s+Date:\n.+`)

func removeDateHeader(str string) string {
	return dateHeaderPattern.ReplaceAllString(str, "")
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
		test := test
		t.Run(name, func(t *testing.T) {
			ctx := context.FromT(t)
			req, body, err := test.req.buildRequest(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(test.expectReq(t), req, cmp.FilterPath(func(path cmp.Path) bool {
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
