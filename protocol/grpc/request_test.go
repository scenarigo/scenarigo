package grpc

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/goccy/go-yaml"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/zoncoen/query-go"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/mockutil"
	"github.com/scenarigo/scenarigo/internal/protocolmeta"
	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/internal/testutil"
	"github.com/scenarigo/scenarigo/internal/yamlutil"
	"github.com/scenarigo/scenarigo/reporter"
	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

func scenarigoMetadata(name string, title string, filepath string) *yamlutil.MDMarshaler {
	md := metadata.MD{}
	if filepath != "" {
		md[protocolmeta.ScenarigoScenarioFilepathBinKey] = []string{filepath}
	}
	if title != "" {
		md[protocolmeta.ScenarigoScenarioTitleBinKey] = []string{title}
	}
	if name != "" {
		md[protocolmeta.ScenarigoStepFullNameBinKey] = []string{name}
	}
	return yamlutil.NewMDMarshaler(md)
}

func TestRequestExtractor(t *testing.T) {
	req := &RequestExtractor{
		Method: "Echo",
		Metadata: metadata.MD{
			"foo": []string{"FOO"},
		},
		Message: &ProtoMessageYAMLMarshaler{
			&testpb.EchoRequest{
				MessageBody: "hey",
			},
		},
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
		"metadata": {
			query:  ".metadata.foo[0]",
			expect: "FOO",
		},
		"message": {
			query:  ".message.messageBody",
			expect: "hey",
		},
		"message (backward compatibility)": {
			query:  ".messageBody",
			expect: "hey",
		},
		"not found": {
			query:       ".message.aaa",
			expectError: `".message.aaa" not found`,
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
		Header: &yamlutil.MDMarshaler{
			"foo": []string{"FOO"},
		},
		Trailer: &yamlutil.MDMarshaler{
			"bar": []string{"BAR"},
		},
		Message: &ProtoMessageYAMLMarshaler{&testpb.EchoResponse{
			MessageBody: "hey",
		}},
	}
	tests := map[string]struct {
		query       string
		expect      any
		expectError string
	}{
		"status": {
			query:  ".status.code",
			expect: resp.Status.Code().String(),
		},
		"header": {
			query:  ".header.foo[0]",
			expect: "FOO",
		},
		"trailer": {
			query:  ".trailer.bar[0]",
			expect: "BAR",
		},
		"message": {
			query:  ".message.messageBody",
			expect: "hey",
		},
		"message (backward compatibility)": {
			query:  ".messageBody",
			expect: "hey",
		},
		"not found": {
			query:       ".message.aaa",
			expectError: `".message.aaa" not found`,
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

func TestRequest_Invoke(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Run("custom client", func(t *testing.T) {
			t.Run("Echo returns no error", func(t *testing.T) {
				req := &testpb.EchoRequest{MessageId: "1", MessageBody: "hello"}
				resp := &testpb.EchoResponse{MessageId: "1", MessageBody: "hello"}

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				client := testpb.NewMockTestClient(ctrl)
				client.EXPECT().Echo(gomock.Any(), mockutil.ProtoMessage(req), gomock.Any()).Return(resp, nil)

				r := &Request{
					Client: "{{vars.client}}",
					Method: "Echo",
					Message: yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "1"},
						yaml.MapItem{Key: "messageBody", Value: "hello"},
					},
				}
				ctx := context.FromT(t).WithVars(map[string]any{
					"client": client,
				})
				ctx, result, err := r.Invoke(ctx)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				typedResult, ok := result.(*response)
				if !ok {
					t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
				}
				if diff := cmp.Diff(resp, typedResult.Message, protocmp.Transform()); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
				if typedResult.Status.Code() != codes.OK {
					t.Fatalf("unexpected error: %v", typedResult.Status.Err())
				}

				// ensure that ctx.WithRequest and ctx.WithResponse are called
				dumpReq := &request{
					Method:   r.Method,
					Metadata: scenarigoMetadata(t.Name(), "", ""),
					Message:  &ProtoMessageYAMLMarshaler{req},
				}
				if diff := cmp.Diff((*RequestExtractor)(dumpReq), ctx.Request(), protocmp.Transform()); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
				if diff := cmp.Diff((*ResponseExtractor)(typedResult), ctx.Response(), protocmp.Transform(), cmpopts.IgnoreUnexported(status.Status{})); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
			})
			t.Run("Echo returns error", func(t *testing.T) {
				req := &testpb.EchoRequest{MessageId: "1", MessageBody: "hello"}

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				client := testpb.NewMockTestClient(ctrl)
				client.EXPECT().Echo(gomock.Any(), mockutil.ProtoMessage(req), gomock.Any()).Return(nil, status.New(codes.Unauthenticated, "unauthenticated").Err())

				r := &Request{
					Client: "{{vars.client}}",
					Method: "Echo",
					Message: yaml.MapSlice{
						yaml.MapItem{Key: "messageId", Value: "1"},
						yaml.MapItem{Key: "messageBody", Value: "hello"},
					},
				}
				ctx := context.FromT(t).WithVars(map[string]any{
					"client": client,
				})
				ctx, result, err := r.Invoke(ctx)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				typedResult, ok := result.(*response)
				if !ok {
					t.Fatalf("failed to type conversion from %s to *response", reflect.TypeOf(result))
				}
				serr := typedResult.Status
				if serr == nil {
					t.Fatal("no error")
				}
				if serr.Code() != codes.Unauthenticated {
					t.Fatalf("expected code is %s but got %s", codes.Unauthenticated.String(), serr.Code().String())
				}

				// ensure that ctx.WithRequest and ctx.WithResponse are called
				dumpReq := &request{
					Method:   r.Method,
					Metadata: scenarigoMetadata(t.Name(), "", ""),
					Message:  &ProtoMessageYAMLMarshaler{req},
				}
				if diff := cmp.Diff((*RequestExtractor)(dumpReq), ctx.Request(), protocmp.Transform()); diff != "" {
					t.Errorf("differs: (-want +got)\n%s", diff)
				}
			})
		})
	})
	t.Run("metadata encoding", func(t *testing.T) {
		req := &testpb.EchoRequest{MessageId: "1", MessageBody: "hello"}
		resp := &testpb.EchoResponse{MessageId: "1", MessageBody: "hello"}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := testpb.NewMockTestClient(ctrl)
		client.EXPECT().Echo(gomock.Any(), mockutil.ProtoMessage(req), gomock.Any()).Return(resp, nil)

		r := &Request{
			Client: "{{vars.client}}",
			Method: "Echo",
			Message: yaml.MapSlice{
				yaml.MapItem{Key: "messageId", Value: "1"},
				yaml.MapItem{Key: "messageBody", Value: "hello"},
			},
		}
		ctx := context.FromT(t).
			WithVars(map[string]any{"client": client}).
			WithScenarioTitle("シナリオ").
			WithScenarioFilepath("テスト/シナリオ.yaml")
		ctx, _, err := r.Invoke(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		dumpReq := &request{
			Method:   r.Method,
			Metadata: scenarigoMetadata(t.Name(), "シナリオ", "テスト/シナリオ.yaml"),
			Message:  &ProtoMessageYAMLMarshaler{req},
		}
		if diff := cmp.Diff((*RequestExtractor)(dumpReq), ctx.Request(), protocmp.Transform()); diff != "" {
			t.Errorf("differs: (-want +got)\n%s", diff)
		}
	})

	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			vars        map[string]any
			client      string
			method      string
			metadata    any
			msg         any
			expectError string
		}{
			"client not found": {
				client:      "{{vars.client}}",
				expectError: "failed to get client",
			},
			"invalid client": {
				vars: map[string]any{
					"client": nil,
				},
				client:      "{{vars.client}}",
				method:      "Echo",
				expectError: `.client: client "{{vars.client}}" is invalid`,
			},
			"invalid metadata: invalid template": {
				vars: map[string]any{
					"client": testpb.NewTestClient(nil),
				},
				method:      "Echo",
				client:      "{{vars.client}}",
				metadata:    map[string]any{"a": "{{b}}"},
				expectError: `.metadata.'a': failed to set metadata: failed to execute: {{b}}: ".b" not found`,
			},
			"invalid metadata: must be map": {
				vars: map[string]any{
					"client": testpb.NewTestClient(nil),
				},
				method:      "Echo",
				client:      "{{vars.client}}",
				metadata:    1,
				expectError: ".metadata: failed to set metadata: expected map but got int",
			},
			"invalid message": {
				vars: map[string]any{
					"client": testpb.NewTestClient(nil),
				},
				method:      "Echo",
				client:      "{{vars.client}}",
				msg:         "test",
				expectError: `.message: failed to build request message`,
			},
		}
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				ctx := context.FromT(t)
				if tc.vars != nil {
					ctx = ctx.WithVars(tc.vars)
				}
				req := &Request{
					Client:  tc.client,
					Method:  tc.method,
					Message: tc.msg,
				}
				if tc.metadata != nil {
					req.Metadata = tc.metadata
				}
				_, _, err := req.Invoke(ctx)
				if err == nil {
					t.Fatal("no error")
				}
				if e := err.Error(); !strings.Contains(e, tc.expectError) {
					t.Errorf(`"%s" does not contain "%s"`, e, tc.expectError)
				}
			})
		}
	})
}

func TestRequest_Invoke_Log(t *testing.T) {
	req := &testpb.EchoRequest{MessageId: "1", MessageBody: "hello"}
	resp := &testpb.EchoResponse{MessageId: "1", MessageBody: "hello"}

	tests := map[string]struct {
		err    error
		expect string
	}{
		"success": {
			expect: `
=== RUN   test.yaml
--- PASS: test.yaml (0.00s)
        request:
          method: Echo
          metadata:
            scenarigo-step-full-name-bin:
            - test.yaml
            version:
            - 1.0.0
          message:
            messageId: "1"
            messageBody: hello
        response:
          status:
            code: OK
          message:
            messageId: "1"
            messageBody: hello
PASS
ok  	test.yaml	0.000s
`,
		},
		"failure": {
			err: createStatus(
				t, codes.InvalidArgument, "invalid argument",
				&errdetails.LocalizedMessage{
					Locale:  "ja-JP",
					Message: "エラー",
				},
				&errdetails.DebugInfo{
					Detail: "debug",
				},
			).Err(),
			expect: `
=== RUN   test.yaml
--- PASS: test.yaml (0.00s)
        request:
          method: Echo
          metadata:
            scenarigo-step-full-name-bin:
            - test.yaml
            version:
            - 1.0.0
          message:
            messageId: "1"
            messageBody: hello
        response:
          status:
            code: InvalidArgument
            message: invalid argument
            details:
              google.rpc.LocalizedMessage:
                locale: ja-JP
                message: エラー
              google.rpc.DebugInfo:
                detail: debug
          message:
            messageId: "1"
            messageBody: hello
PASS
ok  	test.yaml	0.000s
`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := testpb.NewMockTestClient(ctrl)
			client.EXPECT().Echo(gomock.Any(), mockutil.ProtoMessage(req), gomock.Any()).Return(resp, test.err)

			r := &Request{
				Client: "{{vars.client}}",
				Method: "Echo",
				Metadata: map[string]string{
					"version": "1.0.0",
				},
				Message: yaml.MapSlice{
					yaml.MapItem{Key: "messageId", Value: "1"},
					yaml.MapItem{Key: "messageBody", Value: "hello"},
				},
			}

			var b bytes.Buffer
			reporter.Run(func(rptr reporter.Reporter) {
				rptr.Run("test.yaml", func(rptr reporter.Reporter) {
					ctx := context.New(rptr).WithVars(map[string]any{
						"client": client,
					})
					if _, _, err := r.Invoke(ctx); err != nil {
						t.Fatalf("unexpected error: %s", err)
					}
				})
			}, reporter.WithWriter(&b), reporter.WithVerboseLog())

			expect := strings.TrimPrefix(test.expect, "\n")
			if diff := cmp.Diff(expect, testutil.ResetDuration(b.String())); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildRequestBody(t *testing.T) {
	tests := map[string]struct {
		vars   any
		src    any
		expect *testpb.EchoRequest
		error  bool
	}{
		"empty": {
			expect: &testpb.EchoRequest{},
		},
		"set fields": {
			src: yaml.MapSlice{
				yaml.MapItem{
					Key:   "messageId",
					Value: "1",
				},
				yaml.MapItem{
					Key:   "messageBody",
					Value: "hello",
				},
			},
			expect: &testpb.EchoRequest{
				MessageId:   "1",
				MessageBody: "hello",
			},
		},
		"with vars": {
			vars: map[string]string{
				"body": "hello",
			},
			src: yaml.MapSlice{
				yaml.MapItem{
					Key:   "messageBody",
					Value: "{{vars.body}}",
				},
			},
			expect: &testpb.EchoRequest{
				MessageBody: "hello",
			},
		},
		"unknown field": {
			src: yaml.MapSlice{
				yaml.MapItem{
					Key: "invalid",
				},
			},
			error: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.FromT(t)
			if tc.vars != nil {
				ctx = ctx.WithVars(tc.vars)
			}
			var req testpb.EchoRequest
			err := buildRequestMsg(ctx, &req, tc.src)
			if err != nil {
				if !tc.error {
					t.Fatalf("unexpected error: %s", err)
				}
				return
			}
			if tc.error {
				t.Fatal("no error")
			}
			if diff := cmp.Diff(tc.expect, &req, protocmp.Transform()); diff != "" {
				t.Errorf("differs: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestMDMarshaler_MarshalYAML(t *testing.T) {
	tests := map[string]struct {
		md       metadata.MD
		expected string
	}{
		"nil": {
			expected: `method: Foo
metadata: {}
`,
		},
		"empty": {
			md: metadata.MD{},
			expected: `method: Foo
metadata: {}
`,
		},
		"no -bin": {
			md: metadata.MD{
				"grpc-status": {codes.Internal.String()},
			},
			expected: `method: Foo
metadata:
  grpc-status:
  - Internal
`,
		},
		"has -bin": {
			md: metadata.MD{
				"grpc-status-details-bin": {"test", string("\xF4\x90\x80\x80")}, // U+10FFFF+1; out of range

			},
			expected: `method: Foo
metadata:
  grpc-status-details-bin:
  - test
  - f4908080
`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			b, err := yaml.Marshal(Request{
				Method:   "Foo",
				Metadata: yamlutil.NewMDMarshaler(test.md),
			})
			if err != nil {
				t.Fatal(err)
			}
			if got, expected := string(b), test.expected; got != expected {
				dmp := diffmatchpatch.New()
				diffs := dmp.DiffMain(expected, got, false)
				t.Errorf("differs:\n%s", dmp.DiffPrettyText(diffs))
			}
		})
	}
}
