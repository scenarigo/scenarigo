package http

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/zoncoen/scenarigo/context"
)

func TestExpect_Build(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tests := map[string]struct {
			vars     interface{}
			expect   *Expect
			response response
		}{
			"default": {
				expect: &Expect{},
				response: response{
					status: "200 OK",
				},
			},
			"status code": {
				expect: &Expect{
					Code: "404",
				},
				response: response{
					status: "404 Not Found",
				},
			},
			"status code string": {
				expect: &Expect{
					Code: "Not Found",
				},
				response: response{
					status: "404 Not Found",
				},
			},
			"status code (template)": {
				expect: &Expect{
					Code: `{{"Not Found"}}`,
				},
				response: response{
					status: "404 Not Found",
				},
			},
			"header": {
				expect: &Expect{
					Header: yaml.MapSlice{
						{
							Key: "Content-Type",
							Value: []string{
								"application/json",
							},
						},
					},
				},
				response: response{
					Header: map[string][]string{
						"Content-Type": {"application/json"},
					},
					status: "200 OK",
				},
			},
			"assert body": {
				expect: &Expect{
					Body: yaml.MapSlice{
						yaml.MapItem{
							Key:   "foo",
							Value: "bar",
						},
					},
				},
				response: response{
					status: "200 OK",
					Body:   map[string]string{"foo": "bar"},
				},
			},
			"with vars": {
				vars: map[string]string{"foo": "bar"},
				expect: &Expect{
					Body: yaml.MapSlice{
						yaml.MapItem{
							Key:   "foo",
							Value: "{{vars.foo}}",
						},
					},
				},
				response: response{
					status: "200 OK",
					Body:   map[string]string{"foo": "bar"},
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
				assertion, err := test.expect.Build(ctx)
				if err != nil {
					t.Fatalf("failed to build assertion: %s", err)
				}
				if err := assertion.Assert(test.response); err != nil {
					t.Errorf("got assertion error: %s", err)
				}
			})
		}
	})
	t.Run("ng", func(t *testing.T) {
		tests := map[string]struct {
			expect            *Expect
			response          response
			expectBuildError  bool
			expectAssertError bool
		}{
			"wrong status code": {
				expect: &Expect{},
				response: response{
					status: "404 Not Found",
				},
				expectAssertError: true,
			},
			"wrong body": {
				expect: &Expect{
					Body: yaml.MapSlice{
						yaml.MapItem{
							Key:   "foo",
							Value: "bar",
						},
					},
				},
				response: response{
					status: "200 OK",
				},
				expectAssertError: true,
			},
			"wrong header key": {
				expect: &Expect{
					Header: yaml.MapSlice{
						{
							Key:   "invalid-key",
							Value: "value",
						},
					},
				},
				response: response{
					Header: map[string][]string{
						"Content-Type": {
							"application/json",
						},
					},
					status: "200 OK",
				},
				expectAssertError: true,
			},
			"wrong header type": {
				expect: &Expect{
					Header: yaml.MapSlice{
						{
							Key:   1,
							Value: nil,
						},
					},
				},
				response: response{
					Header: map[string][]string{
						"Content-Type": {
							"application/json",
						},
					},
					status: "200 OK",
				},
				expectAssertError: true,
			},
			"failed to execute template": {
				expect: &Expect{
					Body: yaml.MapSlice{
						yaml.MapItem{
							Key:   "foo",
							Value: "{{vars.foo}}",
						},
					},
				},
				expectBuildError: true,
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				ctx := context.FromT(t)
				assertion, err := test.expect.Build(ctx)
				if test.expectBuildError && err == nil {
					t.Fatal("succeeded building assertion")
				}
				if !test.expectBuildError && err != nil {
					t.Fatalf("failed to build assertion: %s", err)
				}
				if err != nil {
					return
				}

				err = assertion.Assert(test.response)
				if test.expectAssertError && err == nil {
					t.Errorf("no assertion error")
				}
				if !test.expectAssertError && err != nil {
					t.Errorf("got assertion error: %s", err)
				}
			})
		}
	})
}
