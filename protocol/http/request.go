package http

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/mattn/go-encoding"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/internal/reflectutil"
	"github.com/scenarigo/scenarigo/protocol/http/marshaler"
	"github.com/scenarigo/scenarigo/protocol/http/unmarshaler"
	"github.com/scenarigo/scenarigo/version"
)

var defaultUserAgent = fmt.Sprintf("scenarigo/%s", version.String())

// Request represents a request.
type Request struct {
	Client string `yaml:"client,omitempty"`
	Method string `yaml:"method,omitempty"`
	URL    string `yaml:"url,omitempty"`
	Query  any    `yaml:"query,omitempty"`
	Header any    `yaml:"header,omitempty"`
	Body   any    `yaml:"body,omitempty"`
}

// RequestExtractor represents a request dump.
type RequestExtractor Request

// ExtractByKey implements query.KeyExtractor interface.
func (r RequestExtractor) ExtractByKey(key string) (any, bool) {
	q := queryutil.New().Key(key)
	if v, err := q.Extract(Request(r)); err == nil {
		return v, true
	}
	// for backward compatibility
	if v, err := q.Extract(r.Body); err == nil {
		return v, true
	}
	return nil, false
}

type response struct {
	Status     string              `yaml:"status,omitempty"` // http.Response.Status format e.g. "200 OK"
	StatusCode int                 `yaml:"statusCode,omitempty"`
	Header     map[string][]string `yaml:"header,omitempty"`
	Body       any                 `yaml:"body,omitempty"`
}

// ResponseExtractor represents a response dump.
type ResponseExtractor response

// ExtractByKey implements query.KeyExtractor interface.
func (r ResponseExtractor) ExtractByKey(key string) (any, bool) {
	q := queryutil.New().Key(key)
	if v, err := q.Extract(response(r)); err == nil {
		return v, true
	}
	// for backward compatibility
	if v, err := q.Extract(r.Body); err == nil {
		return v, true
	}
	return nil, false
}

const (
	indentNum = 2
)

func (r *Request) addIndent(s string, indentNum int) string {
	indent := strings.Repeat(" ", indentNum)
	lines := []string{}
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			lines = append(lines, line)
		} else {
			lines = append(lines, fmt.Sprintf("%s%s", indent, line))
		}
	}
	return strings.Join(lines, "\n")
}

// Invoke implements protocol.Invoker interface.
func (r *Request) Invoke(ctx *context.Context) (*context.Context, any, error) {
	client, err := r.buildClient(ctx)
	if err != nil {
		return ctx, nil, errors.WithPath(err, "client")
	}
	req, reqBody, err := r.buildRequest(ctx)
	if err != nil {
		return ctx, nil, err
	}

	//nolint:exhaustruct
	reqDump := &Request{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: req.Header,
		Body:   reqBody,
	}
	ctx = ctx.WithRequest((*RequestExtractor)(reqDump))
	if b, err := yaml.Marshal(reqDump); err == nil {
		ctx.Reporter().Logf("request:\n%s", r.addIndent(string(b), indentNum))
	} else {
		ctx.Reporter().Logf("failed to dump request:\n%s", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return ctx, nil, errors.Errorf("failed to send request: %s", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctx, nil, errors.Errorf("failed to read response body: %s", err)
	}

	rvalue := response{
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       nil,
	}
	if len(b) > 0 {
		unmarshaler := unmarshaler.Get(resp.Header.Get("Content-Type"))
		var respBody any
		if err := unmarshaler.Unmarshal(b, &respBody); err != nil {
			return ctx, nil, errors.Errorf("failed to unmarshal response body as %s: %s: %s", unmarshaler.MediaType(), string(b), err)
		}
		rvalue.Body = respBody
	}
	ctx = ctx.WithResponse((*ResponseExtractor)(&rvalue))
	if b, err := yaml.Marshal(rvalue); err == nil {
		ctx.Reporter().Logf("response:\n%s", r.addIndent(string(b), indentNum))
	} else {
		ctx.Reporter().Logf("failed to dump response:\n%s", err)
	}
	return ctx, rvalue, nil
}

func (r *Request) buildClient(ctx *context.Context) (*http.Client, error) {
	client := &http.Client{
		Transport: &charsetRoundTripper{
			base: &encodingRoundTripper{
				base: http.DefaultTransport,
			},
		},
	}
	if r.Client != "" {
		x, err := ctx.ExecuteTemplate(r.Client)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get client")
		}
		var ok bool
		if client, ok = x.(*http.Client); !ok {
			return nil, errors.Errorf(`client must be "*http.Client" but got "%T"`, x)
		}
	}
	return client, nil
}

type charsetRoundTripper struct {
	base http.RoundTripper
}

func (rt *charsetRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		_, params, err := mime.ParseMediaType(strings.Trim(ct, " "))
		if err != nil {
			return nil, errors.Errorf("failed to parse Content-Type response header %q: %s", ct, err)
		}
		if name, ok := params["charset"]; ok {
			enc := encoding.GetEncoding(name)
			if enc == nil {
				return nil, errors.Errorf("failed to decode response body: unknown cahrset %q", name)
			}
			resp.Body = &readCloser{
				Reader: enc.NewDecoder().Reader(resp.Body),
				Closer: resp.Body,
			}
		}
	}
	return resp, err
}

type readCloser struct {
	io.Reader
	io.Closer
}

type encodingRoundTripper struct {
	base http.RoundTripper
}

func (rt *encodingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Add("Accept-Encoding", "gzip")
	}
	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if resp.Header.Get("Content-Encoding") == "gzip" {
		r, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, errors.Errorf("failed to read response body: %s", err)
		}
		resp.Body = r
	}
	return resp, err
}

func (r *Request) buildRequest(ctx *context.Context) (*http.Request, any, error) {
	method := http.MethodGet
	if r.Method != "" {
		method = r.Method
	}

	urlStr, err := r.buildURL(ctx)
	if err != nil {
		return nil, nil, err
	}

	header := http.Header{}
	if r.Header != nil {
		x, err := ctx.ExecuteTemplate(r.Header)
		if err != nil {
			return nil, nil, errors.WrapPathf(err, "header", "failed to set header")
		}
		hdr, err := reflectutil.ConvertStringsMap(reflect.ValueOf(x))
		if err != nil {
			return nil, nil, errors.WrapPathf(err, "header", "failed to set header")
		}
		for k, vs := range hdr {
			for _, v := range vs {
				header.Add(k, v)
			}
		}
	}
	if header.Get("User-Agent") == "" {
		header.Set("User-Agent", defaultUserAgent)
	}

	var reader io.Reader
	var body any
	if r.Body != nil {
		x, err := ctx.ExecuteTemplate(r.Body)
		if err != nil {
			return nil, nil, errors.WrapPathf(err, "body", "failed to create request")
		}
		body = x

		marshaler := marshaler.Get(header.Get("Content-Type"))
		b, err := marshaler.Marshal(body)
		if err != nil {
			return nil, nil, errors.ErrorPathf("body", "failed to marshal request body as %s: %#v: %s", marshaler.MediaType(), body, err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx.RequestContext(), strings.ToUpper(method), urlStr, reader)
	if err != nil {
		return nil, nil, errors.Errorf("failed to create request: %s", err)
	}
	req = req.WithContext(ctx.RequestContext())

	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	return req, body, nil
}

func (r *Request) buildURL(ctx *context.Context) (string, error) {
	x, err := ctx.ExecuteTemplate(r.URL)
	if err != nil {
		return "", errors.WrapPathf(err, "url", "failed to get URL")
	}
	urlStr, ok := x.(string)
	if !ok {
		return "", errors.ErrorPathf("url", `URL must be "string" but got "%T"`, x)
	}

	if r.Query != nil {
		u, err := url.Parse(urlStr)
		if err != nil {
			return "", errors.WrapPathf(err, "url", "invalid url: %s", urlStr)
		}
		query := u.Query()

		x, err := ctx.ExecuteTemplate(r.Query)
		if err != nil {
			return "", errors.WrapPathf(err, "query", "failed to set query")
		}
		q, err := reflectutil.ConvertStringsMap(reflect.ValueOf(x))
		if err != nil {
			return "", errors.WrapPathf(err, "query", "failed to set query")
		}
		for k, vs := range q {
			for _, v := range vs {
				query.Add(k, v)
			}
		}

		u.RawQuery = query.Encode()
		urlStr = u.String()
	}

	return urlStr, nil
}
