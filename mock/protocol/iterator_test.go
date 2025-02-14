package protocol

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
)

func TestMockIterator(t *testing.T) {
	type expect struct {
		protocol string
		expect   interface{}
		response interface{}
	}
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			in      string
			expects []expect
		}{
			"minimal": {
				in: `
- protocol: http
  expect:
    method: POST
    body:
      message: hello
  response:
    code: 200
    body:
      message: hello
`,
				expects: []expect{
					{
						protocol: "http",
						expect: yaml.MapSlice{
							yaml.MapItem{
								Key:   "method",
								Value: "POST",
							},
							yaml.MapItem{
								Key: "body",
								Value: yaml.MapSlice{
									yaml.MapItem{
										Key:   "message",
										Value: "hello",
									},
								},
							},
						},
						response: yaml.MapSlice{
							yaml.MapItem{
								Key:   "code",
								Value: uint64(200),
							},
							yaml.MapItem{
								Key: "body",
								Value: yaml.MapSlice{
									yaml.MapItem{
										Key:   "message",
										Value: "hello",
									},
								},
							},
						},
					},
				},
			},
		}

		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				var mocks []Mock
				if err := yaml.Unmarshal([]byte(test.in), &mocks); err != nil {
					t.Fatalf("failed to unmarshale: %s", err)
				}
				iter := NewMockIterator(mocks)
				for _, expect := range test.expects {
					mock, err := iter.Next()
					if err != nil {
						t.Fatalf("failed to get next: %s", err)
					}
					if mock.Protocol != expect.protocol {
						t.Fatalf("expect protocol %q but got %q", expect.protocol, mock.Protocol)
					}
					var req interface{}
					if err := mock.Expect.Unmarshal(&req); err != nil {
						t.Fatalf("failed to unmarshal expect: %s", err)
					}
					if diff := cmp.Diff(expect.expect, req); diff != "" {
						t.Errorf("differs (-want +got):\n%s", diff)
					}
					var resp interface{}
					if err := mock.Response.Unmarshal(&resp); err != nil {
						t.Fatalf("failed to unmarshal response: %s", err)
					}
					if diff := cmp.Diff(expect.response, resp); diff != "" {
						t.Errorf("differs (-want +got):\n%s", diff)
					}
				}
				if err := iter.Stop(); err != nil {
					t.Fatalf("failed to stop iterator: %s", err)
				}
			})
		}
	})

	t.Run("failure", func(t *testing.T) {
		t.Run("over request", func(t *testing.T) {
			var mocks []Mock
			if err := yaml.Unmarshal([]byte(`
- protocol: http
  response:
    code: 200
`), &mocks); err != nil {
				t.Fatalf("failed to unmarshale: %s", err)
			}
			iter := NewMockIterator(mocks)
			if _, err := iter.Next(); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if _, err := iter.Next(); err == nil {
				t.Fatal("no error")
			} else if got, expect := err.Error(), "no mocks remain"; got != expect {
				t.Errorf("expect %q but got %q", expect, got)
			}
			if err := iter.Stop(); err != nil {
				t.Fatalf("failed to stop iterator: %s", err)
			}
		})
		t.Run("mock remains", func(t *testing.T) {
			var mocks []Mock
			if err := yaml.Unmarshal([]byte(`
- protocol: http
  response:
    code: 200
- protocol: http
  response:
    code: 400
`), &mocks); err != nil {
				t.Fatalf("failed to unmarshale: %s", err)
			}
			iter := NewMockIterator(mocks)
			if _, err := iter.Next(); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if err := iter.Stop(); err == nil {
				t.Fatal("no error")
			} else if got, expect := err.Error(), "last 1 mocks remain"; got != expect {
				t.Errorf("expect %q but got %q", expect, got)
			}
		})
	})
}
