package http

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"

	"github.com/pkg/errors"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
	"github.com/zoncoen/scenarigo/protocol/http/marshaler"
	"github.com/zoncoen/scenarigo/protocol/http/unmarshaler"
	"github.com/zoncoen/scenarigo/version"
	"github.com/zoncoen/yaml"
)

var defaultUserAgent = fmt.Sprintf("scenarigo/%s", version.String())

// Request represents a request.
type Request struct {
	Client string      `yaml:"client,omitempty"`
	Method string      `yaml:"method"`
	URL    string      `yaml:"url"`
	Query  interface{} `yaml:"query,omitempty"`
	Header interface{} `yaml:"header,omitempty"`
	Body   interface{} `yaml:"body,omitempty"`
}

type response struct {
	Header interface{} `yaml:"header,omitempty"`
	Body   interface{} `yaml:"body,omitempty"`
	status string      `yaml:"-"` // http.Response.Status format e.g. "200 OK"
}

// Invoke implements protocol.Invoker interface.
func (r *Request) Invoke(ctx *context.Context) (*context.Context, interface{}, error) {
	client, err := r.buildClient(ctx)
	if err != nil {
		return ctx, nil, err
	}
	req, reqBody, err := r.buildRequest(ctx)
	if err != nil {
		return ctx, nil, err
	}

	ctx = ctx.WithRequest(reqBody)
	if b, err := yaml.Marshal(Request{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: req.Header,
		Body:   reqBody,
	}); err == nil {
		ctx.Reporter().Logf("request:\n%s", string(b))
	} else {
		ctx.Reporter().Logf("failed to dump request:\n%s", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return ctx, nil, errors.Errorf("failed to send request: %s", err)
	}
	defer resp.Body.Close()

	var reader io.ReadCloser
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		var err error
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return ctx, nil, errors.Errorf("failed to read response body: %s", err)
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return ctx, nil, errors.Errorf("failed to read response body: %s", err)
	}

	rvalue := response{
		Header: resp.Header,
		status: resp.Status,
	}
	if len(b) > 0 {
		unmarshaler := unmarshaler.Get(resp.Header.Get("Content-Type"))
		var respBody interface{}
		if err := unmarshaler.Unmarshal(b, &respBody); err != nil {
			return ctx, nil, errors.Errorf("failed to unmarshal response body as %s: %s: %s", unmarshaler.MediaType(), string(b), err)
		}
		rvalue.Body = respBody
		if b, err := yaml.Marshal(rvalue); err == nil {
			ctx.Reporter().Logf("response:\n%s", string(b))
		} else {
			ctx.Reporter().Logf("failed to dump response:\n%s", err)
		}
	}
	ctx = ctx.WithResponse(rvalue)
	return ctx, rvalue, nil
}

func (r *Request) buildClient(ctx *context.Context) (*http.Client, error) {
	client := &http.Client{}
	if r.Client != "" {
		x, err := ctx.ExecuteTemplate(r.Client)
		if err != nil {
			return nil, errors.Errorf("failed to get client: %s", err)
		}
		var ok bool
		if client, ok = x.(*http.Client); !ok {
			return nil, errors.Errorf(`client must be "*http.Client" but got "%T"`, x)
		}
	}
	return client, nil
}

func (r *Request) buildRequest(ctx *context.Context) (*http.Request, interface{}, error) {
	method := http.MethodGet
	if r.Method != "" {
		method = r.Method
	}

	x, err := ctx.ExecuteTemplate(r.URL)
	if err != nil {
		return nil, nil, errors.Errorf("failed to get URL: %s", err)
	}
	urlStr, ok := x.(string)
	if !ok {
		return nil, nil, errors.Errorf(`URL must be "string" but got "%T"`, x)
	}

	if r.Query != nil {
		u, err := url.Parse(urlStr)
		if err != nil {
			return nil, nil, errors.Errorf("invalid url: %s: %s", urlStr, err)
		}
		query := u.Query()

		x, err := ctx.ExecuteTemplate(r.Query)
		if err != nil {
			return nil, nil, errors.Errorf("failed to set query: %s", err)
		}
		q, err := reflectutil.ConvertStringsMap(reflect.ValueOf(x))
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to set query")
		}
		for k, vs := range q {
			vs := vs
			for _, v := range vs {
				query.Add(k, v)
			}
		}

		u.RawQuery = query.Encode()
		urlStr = u.String()
	}

	header := http.Header{}
	if r.Header != nil {
		x, err := ctx.ExecuteTemplate(r.Header)
		if err != nil {
			return nil, nil, errors.Errorf("failed to set header: %s", err)
		}
		hdr, err := reflectutil.ConvertStringsMap(reflect.ValueOf(x))
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to set header")
		}
		for k, vs := range hdr {
			vs := vs
			for _, v := range vs {
				header.Add(k, v)
			}
		}
	}
	if header.Get("User-Agent") == "" {
		header.Set("User-Agent", defaultUserAgent)
	}

	var reader io.Reader
	var body interface{}
	if r.Body != nil {
		x, err := ctx.ExecuteTemplate(r.Body)
		if err != nil {
			return nil, nil, errors.Errorf("failed to create request: %s", err)
		}
		body = x

		marshaler := marshaler.Get(header.Get("Content-Type"))
		b, err := marshaler.Marshal(body)
		if err != nil {
			return nil, nil, errors.Errorf("failed to marshal request body as %s: %#v: %s", marshaler.MediaType(), body, err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, urlStr, reader)
	if err != nil {
		return nil, nil, errors.Errorf("failed to create request: %s", err)
	}
	req = req.WithContext(ctx.RequestContext())

	for k, vs := range header {
		vs := vs
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	return req, body, nil
}
