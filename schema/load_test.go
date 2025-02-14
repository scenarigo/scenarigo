package schema

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/protocol"
	"github.com/scenarigo/scenarigo/protocol/http"
	"github.com/sergi/go-diff/diffmatchpatch"
)

type testProtocol struct {
	name string
	opts any
}

func (p *testProtocol) Name() string { return p.name }

func (p *testProtocol) UnmarshalOption(b []byte) error {
	return yaml.Unmarshal(b, &p.opts)
}

func (p *testProtocol) UnmarshalRequest(b []byte) (protocol.Invoker, error) {
	var r request
	if err := yaml.Unmarshal(b, &r); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (p *testProtocol) UnmarshalExpect(b []byte) (protocol.AssertionBuilder, error) {
	var e expect
	if err := yaml.NewDecoder(bytes.NewBuffer(b), yaml.UseOrderedMap()).Decode(&e); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

type request map[interface{}]interface{}

func (r request) Invoke(ctx *context.Context) (*context.Context, interface{}, error) {
	return ctx, nil, nil
}

type expect map[interface{}]interface{}

func (e expect) Build(ctx *context.Context) (assert.Assertion, error) {
	return assert.Build(ctx.RequestContext(), e)
}

func TestLoadScenarios(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %s", err)
	}
	duration := func(t *testing.T, s string) *Duration {
		t.Helper()
		d, err := time.ParseDuration(s)
		if err != nil {
			t.Fatal(err)
		}
		return (*Duration)(&d)
	}
	p := &testProtocol{
		name: "test",
	}
	protocol.Register(p)
	defer protocol.Unregister(p.Name())

	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			path             string
			opts             []LoadOption
			scenarios        []*Scenario
			assertionBuilder interface{}
		}{
			"valid": {
				path: "testdata/valid.yaml",
				scenarios: []*Scenario{
					{
						Title:       "echo-service",
						Description: "check echo-service",
						Vars:        map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								ID:          "POST-say_01",
								Title:       "POST /say",
								Description: "check to respond same message",
								Vars:        nil,
								Protocol:    "test",
								Request: &request{
									"body": map[string]interface{}{
										"message": "{{vars.message}}",
									},
								},
								Expect: &expect{
									"body": yaml.MapSlice{
										yaml.MapItem{
											Key:   "message",
											Value: "{{request.body.body}}",
										},
									},
								},
							},
							{
								ID:          "POST-say_02",
								Title:       "POST /say",
								Description: "check to respond same message",
								Vars:        nil,
								Protocol:    "test",
								Request: &request{
									"body": map[string]interface{}{
										"message": "{{vars.message}}",
									},
								},
								Expect: &expect{
									"body": yaml.MapSlice{
										yaml.MapItem{
											Key:   "message",
											Value: "{{request.body.body}}",
										},
									},
								},
							},
						},
						filepath: "testdata/valid.yaml",
					},
				},
			},
			"anchor": {
				path: "testdata/valid-anchor.yaml",
				scenarios: []*Scenario{
					{
						Title:       "echo-service",
						Description: "check echo-service",
						Vars:        map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								Title:       "POST /say",
								Description: "check to respond same message",
								Vars:        nil,
								Protocol:    "test",
								Request: &request{
									"body": map[string]interface{}{
										"message": "{{vars.message}}",
									},
								},
								Expect: &expect{
									"body": yaml.MapSlice{
										yaml.MapItem{
											Key:   "message",
											Value: "{{request.body.body}}",
										},
									},
								},
							},
						},
						filepath: "testdata/valid-anchor.yaml",
					},
				},
			},
			"without protocol": {
				path: "testdata/valid-without-protocol.yaml",
				scenarios: []*Scenario{
					{
						Title:       "echo-service",
						Description: "check echo-service",
						Vars:        map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								Include: "./valid.yaml",
							},
						},
						filepath: "testdata/valid-without-protocol.yaml",
					},
				},
			},
			"without expect": {
				path: "testdata/valid-without-expect.yaml",
				scenarios: []*Scenario{
					{
						Title:       "echo-service",
						Description: "check echo-service",
						Vars:        map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								Title:       "POST /say",
								Description: "check to respond same message",
								Vars:        nil,
								Protocol:    "test",
								Request: &request{
									"body": map[string]interface{}{
										"message": "{{vars.message}}",
									},
								},
							},
						},
						filepath: "testdata/valid-without-expect.yaml",
					},
				},
			},
			"ytt (single file)": {
				path: "testdata/ytt/single.yaml",
				opts: []LoadOption{
					WithInputConfig(wd, InputConfig{
						YAML: YAMLInputConfig{
							YTT: YTTConfig{
								Enabled: true,
							},
						},
					}),
				},
				scenarios: []*Scenario{
					{
						SchemaVersion: "scenario/v1",
						Title:         "echo",
						Vars:          map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								Title:    "POST /say",
								Vars:     nil,
								Protocol: "test",
								Request: &request{
									"body": map[string]interface{}{
										"message": "{{vars.message}}",
									},
								},
								Expect: &expect{
									"body": yaml.MapSlice{
										yaml.MapItem{
											Key:   "message",
											Value: "{{request.body.message}}",
										},
									},
								},
							},
						},
						filepath: "testdata/ytt/single.yaml",
					},
				},
			},
			"ytt (ytt/v1)": {
				path: "testdata/ytt/scenario.yaml",
				opts: []LoadOption{
					WithInputConfig(wd, InputConfig{
						YAML: YAMLInputConfig{
							YTT: YTTConfig{
								Enabled: true,
								DefaultFiles: []string{
									"testdata/ytt/_ytt_lib",
									"testdata/ytt/default.yaml",
								},
							},
						},
					}),
				},
				scenarios: []*Scenario{
					{
						SchemaVersion: "scenario/v1",
						Title:         "1",
						Vars:          map[string]interface{}{"message": "hellohello"},
						Steps: []*Step{
							{
								Title:    "POST /say",
								Vars:     nil,
								Protocol: "test",
								Request: &request{
									"body": map[string]interface{}{
										"message": "{{vars.message}}",
									},
								},
								Expect: &expect{
									"body": yaml.MapSlice{
										yaml.MapItem{
											Key:   "message",
											Value: "{{request.body.message}}",
										},
									},
								},
								Timeout: duration(t, "30s"),
							},
						},
						filepath: "testdata/ytt/scenario.yaml",
					},
					{
						SchemaVersion: "scenario/v1",
						Title:         "2",
						Vars:          map[string]interface{}{"message": "worldworld"},
						Steps: []*Step{
							{
								Title:    "POST /say",
								Vars:     nil,
								Protocol: "test",
								Request: &request{
									"body": map[string]interface{}{
										"message": "{{vars.message}}",
									},
								},
								Expect: &expect{
									"body": yaml.MapSlice{
										yaml.MapItem{
											Key:   "message",
											Value: "{{request.body.message}}",
										},
									},
								},
								Timeout: duration(t, "10s"),
							},
						},
						filepath: "testdata/ytt/scenario.yaml",
					},
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := LoadScenarios(test.path, test.opts...)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.scenarios, got,
					cmp.AllowUnexported(
						Scenario{},
					),
					cmp.FilterPath(func(path cmp.Path) bool {
						s := path.String()
						return s == "Node"
					}, cmp.Ignore()),
				); diff != "" {
					t.Errorf("scenario differs (-want +got):\n%s", diff)
				}
				for i, scn := range got {
					if g, e := scn.filepath, test.path; g != e {
						t.Errorf("[%d] expect %q but got %q", i, e, g)
					}
					if scn.Node == nil {
						t.Errorf("[%d] Node is nil", i)
					}
				}
			})
		}
	})

	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			path   string
			opts   []LoadOption
			expect string
		}{
			"invalid": {
				path: "testdata/invalid.yaml",
				expect: `failed to decode YAML: [1:8] cannot unmarshal yaml.MapSlice into Go struct field Scenario.Title of type string
>  1 | title: {}
              ^
`,
			},
			"unknown schema version": {
				path: "testdata/unknown-schema-version.yaml",
				expect: `unknown version "scenario/unknown"
    >  1 | schemaVersion: scenario/unknown
                          ^
       2 | title: echo-service
       3 | description: check echo-service
       4 | vars:
`,
			},
			"unknown protocol": {
				path:   "testdata/unknown-protocol.yaml",
				expect: "failed to decode YAML: unknown protocol: unknown",
			},
			"validation error: invalid step id": {
				path: "testdata/invalid-step-id.yaml",
				expect: `validation error: testdata/invalid-step-id.yaml: step id must contain only alphanumeric characters, -, or _
       3 | vars:
       4 |   message: hello
       5 | steps:
    >  6 | - id: POST say
                 ^
       7 |   title: POST /say
       8 |   description: check to respond same message
       9 |   protocol: test
      10 |
`,
			},
			"validation error: dup step id": {
				path: "testdata/invalid-dup-step-id.yaml",
				expect: `validation error: testdata/invalid-dup-step-id.yaml: step id "POST-say" is duplicated
      13 |   expect:
      14 |     body:
      15 |       message: "{{request.body.body}}"
    > 16 | - id: POST-say
                 ^
      17 |   title: POST /say
      18 |   description: check to respond same message
      19 |   protocol: test
      20 |
`,
			},
			"validation error: no protocol": {
				path: "testdata/invalid-no-protocol.yaml",
				expect: `validation error: testdata/invalid-no-protocol.yaml: no protocol
       1 | title: test
       2 | steps:
    >  3 | - title: foo
                  ^
`,
			},
			"validation error: unknown protocol": {
				path: "testdata/invalid-unknown-protocol.yaml",
				expect: `validation error: testdata/invalid-unknown-protocol.yaml: protocol "aaa" not found
       1 | title: test
       2 | steps:
       3 | - title: foo
    >  4 |   protocol: aaa
                       ^
`,
			},
			"ytt disabled": {
				path: "testdata/ytt/scenario.yaml",
				expect: `ytt feature is not enabled
    >  1 | schemaVersion: ytt/v1
                          ^
       2 | files:
       3 | - template.ytt.yaml
       4 | - values.ytt.yaml
`,
			},
			"ytt file not found": {
				path: "testdata/ytt/invalid.yaml",
				opts: []LoadOption{
					WithInputConfig(wd, InputConfig{
						YAML: YAMLInputConfig{
							YTT: YTTConfig{
								Enabled: true,
							},
						},
					}),
				},
				expect: fmt.Sprintf("failed to read ytt files: lstat %s/testdata/ytt/not-found.ytt.yaml: no such file or directory", wd),
			},
			"default ytt file not found": {
				path: "testdata/ytt/scenario.yaml",
				opts: []LoadOption{
					WithInputConfig(wd, InputConfig{
						YAML: YAMLInputConfig{
							YTT: YTTConfig{
								Enabled: true,
								DefaultFiles: []string{
									"testdata/ytt/not-found.yaml",
								},
							},
						},
					}),
				},
				expect: fmt.Sprintf("failed to read default ytt files: lstat %s/testdata/ytt/not-found.yaml: no such file or directory", wd),
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				_, err := LoadScenarios(test.path, test.opts...)
				if err == nil {
					t.Fatal("expected error but no error")
				}
				if got, expect := err.Error(), test.expect; got != expect {
					// t.Errorf("expect %q but got %q", expect, got)
					t.Errorf("\n=== expect ===\n%s\n=== got ===\n%s\n", test.expect, got)
				}
			})
		}
	})
}

func TestLoadScenariosFromReader(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			yaml      string
			scenarios []*Scenario
		}{
			"valid": {
				yaml: `
title: echo-service
description: check echo-service
vars:
  message: hello
steps:
  - title: POST /say
    description: check to respond same message
    protocol: test
    request:
      body:
        message: "{{vars.message}}"
    expect:
      body:
        message: "{{request.body.body}}"
`,
				scenarios: []*Scenario{
					{
						Title:       "echo-service",
						Description: "check echo-service",
						Vars:        map[string]interface{}{"message": "hello"},
						Steps: []*Step{
							{
								Title:       "POST /say",
								Description: "check to respond same message",
								Vars:        nil,
								Protocol:    "test",
								Request: &request{
									"body": map[string]interface{}{
										"message": "{{vars.message}}",
									},
								},
								Expect: &expect{
									"body": yaml.MapSlice{
										yaml.MapItem{
											Key:   "message",
											Value: "{{request.body.body}}",
										},
									},
								},
							},
						},
						filepath: filepath.Join(wd, "reader.yaml"),
					},
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				p := &testProtocol{
					name: "test",
				}
				protocol.Register(p)
				defer protocol.Unregister(p.Name())

				got, err := LoadScenariosFromReader(strings.NewReader(test.yaml))
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.scenarios, got,
					cmp.AllowUnexported(
						Scenario{},
					),
					cmp.FilterPath(func(path cmp.Path) bool {
						s := path.String()
						return s == "Node"
					}, cmp.Ignore()),
				); diff != "" {
					t.Errorf("scenario differs (-want +got):\n%s", diff)
				}
				for i, scn := range got {
					if scn.Node == nil {
						t.Errorf("[%d] Node is nil", i)
					}
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			r io.Reader
		}{
			"failed to read": {
				r: errReader{errors.New("read error")},
			},
			"parse error": {
				r: strings.NewReader(`
a:
- b
  c: d`),
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				_, err := LoadScenariosFromReader(test.r)
				if err == nil {
					t.Fatal("expected error but no error")
				}
			})
		}
	})
}

type errReader struct {
	err error
}

func (r errReader) Read(_ []byte) (int, error) { return 0, r.err }

func TestMarshalYAML(t *testing.T) {
	filename := "testdata/valid.yaml"

	p := &testProtocol{
		name: "test",
	}
	protocol.Register(p)
	defer protocol.Unregister(p.Name())

	scenarios, err := LoadScenarios(filename)
	if err != nil {
		t.Fatalf("failed to load scenarios: %s", err)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	for _, s := range scenarios {
		if err := enc.Encode(s); err != nil {
			t.Fatalf("failed to marshal to YAML: %s", err)
		}
	}

	b, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %s", err)
	}

	if got, expect := buf.String(), string(b); got != expect {
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(expect, got, false)
		t.Errorf("differs:\n%s", dmp.DiffPrettyText(diffs))
	}
}

func Test_Issue304(t *testing.T) {
	http.Register()
	yml := `title: get scenarigo repository
steps:
- title: expect 404
  protocol: http
  request:
    method: GET
    url: https://api.github.com/repos/zoncoen/aaaaaaaaaaaaa
    body:
      # message: this comment does evil
  expect:
    code: Not Found
`
	scenarios, err := LoadScenariosFromReader(strings.NewReader(yml))
	if err != nil {
		t.Fatal(err)
	}
	if l := len(scenarios); l != 1 {
		t.Fatalf("unexpected length %d", l)
	}
	b, err := yaml.Marshal(scenarios[0])
	if err != nil {
		t.Fatal(err)
	}

	expect := `title: get scenarigo repository
steps:
- title: expect 404
  protocol: http
  request:
    method: GET
    url: https://api.github.com/repos/zoncoen/aaaaaaaaaaaaa
  expect:
    code: Not Found
`
	if got := string(b); got != expect {
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(expect, got, false)
		t.Errorf("differs:\n%s", dmp.DiffPrettyText(diffs))
	}
}
