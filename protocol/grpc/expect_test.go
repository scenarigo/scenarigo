package grpc

import (
	"strconv"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"

	"github.com/goccy/go-yaml"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/yamlutil"
	"github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

func TestExpect_Build(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tests := map[string]struct {
			vars   interface{}
			expect *Expect
			v      *response
		}{
			"default": {
				expect: &Expect{},
				v: &response{
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
			},
			"code": {
				expect: &Expect{
					Code: strconv.Itoa(int(codes.InvalidArgument)),
				},
				v: &response{
					Status: createStatus(t, codes.InvalidArgument, "invalid argument"),
				},
			},
			"code string": {
				expect: &Expect{
					Code: "InvalidArgument",
				},
				v: &response{
					Status: createStatus(t, codes.InvalidArgument, "invalid argument"),
				},
			},
			"code template string": {
				expect: &Expect{
					Code: `{{"InvalidArgument"}}`,
				},
				v: &response{
					Status: createStatus(t, codes.InvalidArgument, "invalid argument"),
				},
			},
			"assert body": {
				expect: &Expect{
					Code: "OK",
					Message: yaml.MapSlice{
						yaml.MapItem{
							Key:   "messageId",
							Value: "1",
						},
						yaml.MapItem{
							Key:   "messageBody",
							Value: "hello",
						},
					},
				},
				v: &response{
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{
						MessageId:   "1",
						MessageBody: "hello",
					}},
				},
			},
			"assert metadata.header": {
				expect: &Expect{
					Code: "OK",
					Header: yaml.MapSlice{
						{
							Key:   "content-type",
							Value: "application/grpc",
						},
					},
				},
				v: &response{
					Header: yamlutil.NewMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
			},
			"assert metadata.trailer": {
				expect: &Expect{
					Code: "OK",
					Trailer: yaml.MapSlice{
						{
							Key:   "content-type",
							Value: "application/grpc",
						},
					},
				},
				v: &response{
					Trailer: yamlutil.NewMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
			},
			"assert in case of error": {
				expect: &Expect{
					Status: ExpectStatus{
						Code:    `{{"InvalidArgument"}}`,
						Message: `{{"invalid argument"}}`,
						Details: []map[string]yaml.MapSlice{
							{
								"google.rpc.LocalizedMessage": yaml.MapSlice{
									yaml.MapItem{
										Key:   "locale",
										Value: "ja-JP",
									},
								},
							},
							{
								`{{"google.rpc.DebugInfo"}}`: yaml.MapSlice{
									yaml.MapItem{
										Key:   `{{"detail"}}`,
										Value: "debug",
									},
								},
							},
						},
					},
				},
				v: &response{
					Status: createStatus(
						t, codes.InvalidArgument, "invalid argument",
						&errdetails.LocalizedMessage{
							Locale:  "ja-JP",
							Message: "エラー",
						},
						&errdetails.DebugInfo{
							Detail: "debug",
						},
					),
				},
			},
			"assert in case of error with template string": {
				expect: &Expect{
					Status: ExpectStatus{
						Code:    `{{"InvalidArgument"}}`,
						Message: `{{"invalid argument"}}`,
					},
				},
				v: &response{
					Status: createStatus(t, codes.InvalidArgument, "invalid argument"),
				},
			},
			"with vars": {
				vars: map[string]string{"body": "hello"},
				expect: &Expect{
					Code: "OK",
					Message: yaml.MapSlice{
						yaml.MapItem{
							Key:   "messageId",
							Value: "1",
						},
						yaml.MapItem{
							Key:   "messageBody",
							Value: "{{vars.body}}",
						},
					},
				},
				v: &response{
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{
						MessageId:   "1",
						MessageBody: "hello",
					}},
				},
			},
			"with $": {
				vars: map[string]string{"body": "hello"},
				expect: &Expect{
					Status: ExpectStatus{
						Code:    `{{$ == "InvalidArgument"}}`,
						Message: `{{$ == "invalid argument"}}`,
						Details: []map[string]yaml.MapSlice{
							{
								`{{$ == "google.rpc.LocalizedMessage"}}`: yaml.MapSlice{
									{
										Key:   "locale",
										Value: `{{$ == "ja-JP"}}`,
									},
									{
										Key:   "message",
										Value: `{{$ == "エラー"}}`,
									},
								},
							},
						},
					},
					Message: yaml.MapSlice{
						yaml.MapItem{
							Key:   "messageBody",
							Value: `{{$ == vars.body}}`,
						},
					},
					Header: yaml.MapSlice{
						{
							Key:   "content-type",
							Value: `{{$ == "application/grpc"}}`,
						},
					},
					Trailer: yaml.MapSlice{
						{
							Key:   "version",
							Value: `{{$ == "v1.0.0"}}`,
						},
					},
				},
				v: &response{
					Status: createStatus(
						t, codes.InvalidArgument, "invalid argument",
						&errdetails.LocalizedMessage{
							Locale:  "ja-JP",
							Message: "エラー",
						},
					),
					Header: yamlutil.NewMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					Trailer: yamlutil.NewMDMarshaler(metadata.MD{
						"version": []string{
							"v1.0.0",
						},
					}),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{
						MessageBody: "hello",
					}},
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
				if err := assertion.Assert(test.v); err != nil {
					t.Errorf("got assertion error: %s", err)
				}
			})
		}
	})
	t.Run("ng", func(t *testing.T) {
		tests := map[string]struct {
			expect            *Expect
			v                 *response
			expectBuildError  bool
			expectAssertError bool
			expectError       string
		}{
			"failed to execute template": {
				expect: &Expect{
					Code: "OK",
					Message: yaml.MapSlice{
						yaml.MapItem{
							Key:   "messageId",
							Value: "1",
						},
						yaml.MapItem{
							Key:   "messageBody",
							Value: "{{vars.body}}",
						},
					},
				},
				expectBuildError: true,
			},
			"invalid header assertion": {
				expect: &Expect{
					Header: yaml.MapSlice{
						yaml.MapItem{
							Key:   nil,
							Value: "value",
						},
					},
				},
				expectBuildError: true,
			},
			"invalid trailer assertion": {
				expect: &Expect{
					Trailer: yaml.MapSlice{
						yaml.MapItem{
							Key:   nil,
							Value: "value",
						},
					},
				},
				expectBuildError: true,
			},
			"wrong code in case of default": {
				expect: &Expect{},
				v: &response{
					Status:  createStatus(t, codes.InvalidArgument, "invalid argument"),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
			},
			"wrong code": {
				expect: &Expect{
					Code: "OK",
				},
				v: &response{
					Status:  createStatus(t, codes.InvalidArgument, "invalid argument"),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
			},
			"wrong body": {
				expect: &Expect{
					Code: "OK",
					Message: yaml.MapSlice{
						yaml.MapItem{
							Key:   "messageId",
							Value: "1",
						},
						yaml.MapItem{
							Key:   "messageBody",
							Value: "hello",
						},
					},
				},
				v: &response{
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{
						MessageId:   "1",
						MessageBody: "hell",
					}},
				},
				expectAssertError: true,
			},
			"invalid type of metadata.header": {
				expect: &Expect{
					Code: "OK",
					Header: yaml.MapSlice{
						{
							Key:   "invalid_key",
							Value: nil,
						},
					},
				},
				v: &response{
					Header: yamlutil.NewMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
			},
			"wrong metadata.header key": {
				expect: &Expect{
					Code: "OK",
					Header: yaml.MapSlice{
						{
							Key:   "invalid_key",
							Value: "",
						},
					},
				},
				v: &response{
					Header: yamlutil.NewMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
			},
			"wrong metadata.header value": {
				expect: &Expect{
					Code: "OK",
					Header: yaml.MapSlice{
						{
							Key:   "content-type",
							Value: "invalid_value",
						},
					},
				},
				v: &response{
					Header: yamlutil.NewMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
			},
			"wrong metadata.trailer key": {
				expect: &Expect{
					Code: "OK",
					Trailer: yaml.MapSlice{
						{
							Key:   "invalid_key",
							Value: "",
						},
					},
				},
				v: &response{
					Trailer: yamlutil.NewMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
			},
			"wrong metadata.trailer value": {
				expect: &Expect{
					Code: "OK",
					Trailer: yaml.MapSlice{
						{
							Key:   "content-type",
							Value: "invalid_value",
						},
					},
				},
				v: &response{
					Trailer: yamlutil.NewMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
			},
			"wrong status code": {
				expect: &Expect{
					Status: ExpectStatus{
						Code: "Invalid Argument",
					},
				},
				v: &response{
					Status:  createStatus(t, codes.NotFound, "not found"),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
			},
			"wrong status message": {
				expect: &Expect{
					Status: ExpectStatus{
						Code:    "NotFound",
						Message: "foo",
					},
				},
				v: &response{
					Status:  createStatus(t, codes.NotFound, "not found"),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
			},

			// status details
			"wrong status details: type name is an invalid template": {
				expect: &Expect{
					Status: ExpectStatus{
						Details: []map[string]yaml.MapSlice{
							{
								"{{google.rpc.LocalizedMessage": yaml.MapSlice{
									yaml.MapItem{
										Key:   "locale",
										Value: "ja-JP",
									},
								},
							},
						},
					},
				},
				v: &response{
					Status: createStatus(
						t, codes.InvalidArgument, "invalid argument",
						&errdetails.LocalizedMessage{
							Locale:  "ja-JP",
							Message: "エラー",
						},
						&errdetails.DebugInfo{
							Detail: "debug",
						},
					),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectBuildError: true,
			},
			"wrong status details: type name is wrong": {
				expect: &Expect{
					Status: ExpectStatus{
						Code: "InvalidArgument",
						Details: []map[string]yaml.MapSlice{
							{
								"google.rpc.Invalid": yaml.MapSlice{
									yaml.MapItem{
										Key:   "detail",
										Value: "debug",
									},
								},
							},
						},
					},
				},
				v: &response{
					Status: createStatus(
						t, codes.InvalidArgument, "invalid argument",
						&errdetails.LocalizedMessage{
							Locale:  "ja-JP",
							Message: "エラー",
						},
						&errdetails.DebugInfo{
							Detail: "debug",
						},
					),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
				expectError:       `.status.details[0]: expected "google.rpc.Invalid" but got "google.rpc.LocalizedMessage"`,
			},
			"wrong status details: key is an invalid template": {
				expect: &Expect{
					Status: ExpectStatus{
						Code: "InvalidArgument",
						Details: []map[string]yaml.MapSlice{
							{
								"google.rpc.LocalizedMessage": yaml.MapSlice{
									yaml.MapItem{
										Key:   "{{locale",
										Value: "ja-JP",
									},
								},
							},
						},
					},
				},
				v: &response{
					Status: createStatus(
						t, codes.InvalidArgument, "invalid argument",
						&errdetails.LocalizedMessage{
							Locale:  "ja-JP",
							Message: "エラー",
						},
						&errdetails.DebugInfo{
							Detail: "debug",
						},
					),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectBuildError: true,
				expectError:      `.status.details[0].'google.rpc.LocalizedMessage': invalid expect status detail: failed to build assertion: failed to parse "{{locale": col 9: expected '}}', found 'EOF'`,
			},
			"wrong status details: key not found": {
				expect: &Expect{
					Status: ExpectStatus{
						Code: "InvalidArgument",
						Details: []map[string]yaml.MapSlice{
							{
								"google.rpc.LocalizedMessage": yaml.MapSlice{
									yaml.MapItem{
										Key:   "Loc",
										Value: "ja-JP",
									},
								},
							},
						},
					},
				},
				v: &response{
					Status: createStatus(
						t, codes.InvalidArgument, "invalid argument",
						&errdetails.LocalizedMessage{
							Locale:  "ja-JP",
							Message: "エラー",
						},
						&errdetails.DebugInfo{
							Detail: "debug",
						},
					),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
				expectError:       `.status.details[0].'google.rpc.LocalizedMessage': ".Loc" not found`,
			},
			"wrong status details: value is wrong": {
				expect: &Expect{
					Status: ExpectStatus{
						Code: "InvalidArgument",
						Details: []map[string]yaml.MapSlice{
							{
								"google.rpc.LocalizedMessage": yaml.MapSlice{
									yaml.MapItem{
										Key:   "Locale",
										Value: "en-US",
									},
								},
							},
						},
					},
				},
				v: &response{
					Status: createStatus(
						t, codes.InvalidArgument, "invalid argument",
						&errdetails.LocalizedMessage{
							Locale:  "ja-JP",
							Message: "エラー",
						},
						&errdetails.DebugInfo{
							Detail: "debug",
						},
					),
					Message: &ProtoMessageYAMLMarshaler{&test.EchoResponse{}},
				},
				expectAssertError: true,
				expectError:       `.status.details[0].'google.rpc.LocalizedMessage'.Locale: expected "en-US" but got "ja-JP"`,
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				ctx := context.FromT(t)
				assertion, err := test.expect.Build(ctx)
				if test.expectBuildError {
					if err == nil {
						t.Fatal("succeeded building assertion")
					}
					if test.expectError != "" {
						if got, expect := err.Error(), test.expectError; got != expect {
							t.Fatalf("expect %q but got %q", expect, got)
						}
					}
				}
				if !test.expectBuildError && err != nil {
					t.Fatalf("failed to build assertion: %s", err)
				}
				if err != nil {
					return
				}

				err = assertion.Assert(test.v)
				if test.expectAssertError {
					if err == nil {
						t.Fatal("no assertion error")
					}
					if test.expectError != "" {
						if got, expect := err.Error(), test.expectError; got != expect {
							t.Fatalf("\nexpect: %s\ngot:    %s", expect, got)
						}
					}
				}
				if !test.expectAssertError && err != nil {
					t.Fatalf("got assertion error: %s", err)
				}
			})
		}
	})
	t.Run("invalid type for assertion.Assert", func(t *testing.T) {
		tests := map[string]struct {
			expect *Expect
			v      interface{}
		}{
			"invalid type for assertion.Assert": {
				expect: &Expect{},
				v:      "string is unexpected value",
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				ctx := context.FromT(t)
				assertion, err := test.expect.Build(ctx)
				if err != nil {
					t.Fatalf("failed to build assertion: %s", err)
				}
				if err := assertion.Assert(test.v); err == nil {
					t.Errorf("no assertion error")
				}
			})
		}
	})
}

func createStatus(t *testing.T, c codes.Code, msg string, details ...proto.Message) *responseStatus {
	t.Helper()
	sts := status.New(c, msg)
	if len(details) > 0 {
		in := make([]protoadapt.MessageV1, len(details))
		for i, d := range details {
			in[i] = protoadapt.MessageV1Of(d)
		}
		var err error
		sts, err = sts.WithDetails(in...)
		if err != nil {
			t.Fatalf("failed to create status with details: %s", err)
		}
	}
	return &responseStatus{sts}
}
