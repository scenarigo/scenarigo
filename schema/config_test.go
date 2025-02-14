package schema

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/scenarigo/scenarigo/protocol"
	"github.com/sergi/go-diff/diffmatchpatch"
)

func TestLoadConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			path           string
			expectComments yaml.CommentMap
		}{
			"without comment": {
				path: "testdata/config/valid.yaml",
			},
			"with comment": {
				path: "testdata/config/valid-with-comment.yaml",
				expectComments: yaml.CommentMap{
					"$.schemaVersion": []*yaml.Comment{
						{
							Texts:    []string{" comment1", " comment2"},
							Position: yaml.CommentHeadPosition,
						},
					},
					"$.plugins.'remote-with-version.so'.src": []*yaml.Comment{
						{
							Texts:    []string{" comment3"},
							Position: yaml.CommentLinePosition,
						},
					},
				},
			},
		}
		re := regexp.MustCompile(".ytt.yaml$")
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				wd, err := os.Getwd()
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				got, err := LoadConfig(test.path)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				colored := true
				expect := &Config{
					SchemaVersion: "config/v1",
					Scenarios: []string{
						"scenarios/a.yaml",
						"scenarios/b.yaml",
					},
					PluginDirectory: "gen",
					Plugins: OrderedMap[string, PluginConfig]{
						idx: map[string]int{
							"local.so":               0,
							"remote.so":              1,
							"remote-with-version.so": 2,
						},
						items: []OrderedMapItem[string, PluginConfig]{
							{
								Key: "local.so",
								Value: PluginConfig{
									Src: "./plugin",
								},
							},
							{
								Key: "remote.so",
								Value: PluginConfig{
									Src: "github.com/scenarigo/scenarigo",
								},
							},
							{
								Key: "remote-with-version.so",
								Value: PluginConfig{
									Src: "github.com/scenarigo/scenarigo@v1.0.0",
								},
							},
						},
					},
					Protocols: ProtocolOptions{
						idx: map[string]int{"grpc": 0},
						items: []OrderedMapItem[string, any]{
							{
								Key: "grpc",
								Value: RawMessage(`request:
  proto:
    imports:
    - proto
  auth:
    insecure: true`),
							},
						},
					},
					Input: InputConfig{
						Excludes: []Regexp{
							{
								Regexp: re,
								str:    ".ytt.yaml$",
							},
						},
						YAML: YAMLInputConfig{
							YTT: YTTConfig{
								Enabled: true,
								DefaultFiles: []string{
									"default.yaml",
								},
							},
						},
					},
					Output: OutputConfig{
						Verbose: true,
						Colored: &colored,
						Report: ReportConfig{
							JSON: JSONReportConfig{
								Filename: "report.json",
							},
							JUnit: JUnitReportConfig{
								Filename: "junit.xml",
							},
						},
					},
					Root:     filepath.Join(wd, "testdata/config"),
					Comments: test.expectComments,
				}
				if got.Node == nil {
					t.Fatalf("node is nil")
				}
				got.Node = nil
				if diff := cmp.Diff(expect, got, cmp.AllowUnexported(Regexp{}, OrderedMap[string, PluginConfig]{}, ProtocolOptions{}), cmpopts.IgnoreUnexported(regexp.Regexp{})); diff != "" {
					t.Errorf("differs (-want +got):\n%s", diff)
				}

				b, err := yaml.MarshalWithOptions(got, yaml.WithComment(got.Comments))
				if err != nil {
					t.Fatalf("failed to marshal: %s", err)
				}
				eb, err := os.ReadFile(test.path)
				if err != nil {
					t.Fatalf("failed to read file: %s", err)
				}
				if got, expect := string(b), string(eb); got != expect {
					dmp := diffmatchpatch.New()
					diffs := dmp.DiffMain(expect, got, false)
					t.Errorf("differs:\n%s", dmp.DiffPrettyText(diffs))
				}
			})
		}
	})

	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			path   string
			expect string
		}{
			"empty": {
				path:   "testdata/config/empty.yaml",
				expect: "empty config",
			},
			"multi document": {
				path:   "testdata/config/multi.yaml",
				expect: "must be a config document but contains more than one document",
			},
			"no version": {
				path:   "testdata/config/no-version.yaml",
				expect: "schemaVersion not found",
			},
			"unknown version": {
				path: "testdata/config/unknown-version.yaml",
				expect: `unknown version "config/unknown"
    >  1 | schemaVersion: config/unknown
                          ^
`,
			},
			"invalid version": {
				path: "testdata/config/invalid-version.yaml",
				expect: `invalid version: [2:3] cannot unmarshal []interface {} into Go value of type string
   1 | schemaVersion:
>  2 |   - config
         ^
   3 |   - v1`,
			},
			"invalid scenarios": {
				path: "testdata/config/invalid-scenarios.yaml",
				expect: `1 error occurred: scenarios/invalid.yaml: no such file or directory
       1 | schemaVersion: config/v1
       2 | scenarios:
    >  3 |   - scenarios/invalid.yaml
               ^
`,
			},
			"plugin src not found": {
				path: "testdata/config/invalid-plugin-src-not-found.yaml",
				expect: `1 error occurred: invalid: no such file or directory
       1 | schemaVersion: config/v1
       2 | plugins:
       3 |   foo.so:
    >  4 |     src: invalid
                    ^
`,
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				_, err := LoadConfig(test.path)
				if err == nil {
					t.Fatal("no error")
				}
				if got := err.Error(); test.expect != got {
					t.Errorf("\n=== expect ===\n%s\n=== got ===\n%s\n", test.expect, got)
				}
			})
		}
	})
}

func TestProtocolOptions_Set(t *testing.T) {
	tests := map[string]struct {
		protocol    protocol.Protocol
		opts        *ProtocolOptions
		expect      protocol.Protocol
		expectError string
	}{
		"success": {
			protocol: &testProtocol{
				name: "test",
			},
			opts: &ProtocolOptions{
				idx: map[string]int{
					"test": 0,
				},
				items: []OrderedMapItem[string, any]{
					{
						Key:   "test",
						Value: RawMessage([]byte("true")),
					},
				},
			},
			expect: &testProtocol{
				name: "test",
				opts: true,
			},
		},
		"unknown protocol": {
			opts: &ProtocolOptions{
				idx: map[string]int{
					"test": 0,
				},
				items: []OrderedMapItem[string, any]{
					{
						Key:   "test",
						Value: RawMessage([]byte("true")),
					},
				},
			},
			expectError: ".protocols.test: unknown protocol",
		},
		"failed to unmarshal": {
			protocol: &testProtocol{
				name: "test",
			},
			opts: &ProtocolOptions{
				idx: map[string]int{
					"test": 0,
				},
				items: []OrderedMapItem[string, any]{
					{
						Key:   "test",
						Value: RawMessage([]byte(":")),
					},
				},
			},
			expectError: ".protocols.test: failed to unmarshal YAML",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.protocol != nil {
				protocol.Register(test.protocol)
				t.Cleanup(func() {
					protocol.Unregister(test.protocol.Name())
				})
			}

			err := test.opts.Set()
			if err != nil {
				if test.expectError == "" {
					t.Fatalf("unexpected error: %s", err)
				}
				if !strings.Contains(err.Error(), test.expectError) {
					t.Fatalf("expect %q but got %q", test.expectError, err)
				}
				return // got expected error
			}
			if test.expectError != "" {
				t.Fatal("no error")
			}
			if diff := cmp.Diff(test.expect, test.protocol, cmp.AllowUnexported(testProtocol{})); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}
