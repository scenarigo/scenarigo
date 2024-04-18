package grpc

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/goccy/go-yaml"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
	"github.com/zoncoen/scenarigo/testdata/gen/pb/test"
)

func TestExpect_Build(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tests := map[string]struct {
			vars   interface{}
			expect *Expect
			v      response
		}{
			"default": {
				expect: &Expect{},
				v: response{
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{}),
						reflect.Zero(reflectutil.TypeError),
					},
				},
			},
			"code": {
				expect: &Expect{
					Code: strconv.Itoa(int(codes.InvalidArgument)),
				},
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
					},
				},
			},
			"code string": {
				expect: &Expect{
					Code: "InvalidArgument",
				},
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
					},
				},
			},
			"code template string": {
				expect: &Expect{
					Code: `{{"InvalidArgument"}}`,
				},
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{
							MessageId:   "1",
							MessageBody: "hello",
						}),
						reflect.Zero(reflectutil.TypeError),
					},
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
				v: response{
					Header: newMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{}),
						reflect.Zero(reflectutil.TypeError),
					},
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
				v: response{
					Trailer: newMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{}),
						reflect.Zero(reflectutil.TypeError),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.FromProto(&spb.Status{
							Code:    int32(codes.InvalidArgument),
							Message: "invalid argument",
							Details: []*anypb.Any{
								mustAny(t,
									&errdetails.LocalizedMessage{
										Locale:  "ja-JP",
										Message: "エラー",
									},
								),
								mustAny(t,
									&errdetails.DebugInfo{
										Detail: "debug",
									},
								),
							},
						}).Err()),
					},
				},
			},
			"assert in case of error with template string": {
				expect: &Expect{
					Status: ExpectStatus{
						Code:    `{{"InvalidArgument"}}`,
						Message: `{{"invalid argument"}}`,
					},
				},
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(
							status.New(codes.InvalidArgument, "invalid argument").Err()),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{
							MessageId:   "1",
							MessageBody: "hello",
						}),
						reflect.Zero(reflectutil.TypeError),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{
							MessageBody: "hello",
						}),
						reflect.ValueOf(status.FromProto(&spb.Status{
							Code:    int32(codes.InvalidArgument),
							Message: "invalid argument",
							Details: []*anypb.Any{
								mustAny(t,
									&errdetails.LocalizedMessage{
										Locale:  "ja-JP",
										Message: "エラー",
									},
								),
							},
						}).Err()),
					},
					Header: newMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					Trailer: newMDMarshaler(metadata.MD{
						"version": []string{
							"v1.0.0",
						},
					}),
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
			v                 response
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

			"return value must be []reflect.Value": {
				expect:            &Expect{},
				v:                 response{},
				expectAssertError: true,
			},
			"the length of return values must be 2": {
				expect: &Expect{},
				v: response{
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{}),
					},
				},
				expectAssertError: true,
			},
			"fist return value must be proto.Message": {
				expect: &Expect{},
				v: response{
					rvalues: []reflect.Value{
						reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
						reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
					},
				},
				expectAssertError: true,
			},
			"second return value must be error": {
				expect: &Expect{},
				v: response{
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{}),
						reflect.ValueOf(&test.EchoResponse{}),
					},
				},
				expectAssertError: true,
			},
			"wrong code in case of default": {
				expect: &Expect{},
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
					},
				},
				expectAssertError: true,
			},
			"wrong code": {
				expect: &Expect{
					Code: "OK",
				},
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{
							MessageId:   "1",
							MessageBody: "hell",
						}),
						reflect.Zero(reflectutil.TypeError),
					},
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
				v: response{
					Header: newMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{}),
						reflect.Zero(reflectutil.TypeError),
					},
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
				v: response{
					Header: newMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{}),
						reflect.Zero(reflectutil.TypeError),
					},
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
				v: response{
					Header: newMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{}),
						reflect.Zero(reflectutil.TypeError),
					},
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
				v: response{
					Trailer: newMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{}),
						reflect.Zero(reflectutil.TypeError),
					},
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
				v: response{
					Trailer: newMDMarshaler(metadata.MD{
						"content-type": []string{
							"application/grpc",
						},
					}),
					rvalues: []reflect.Value{
						reflect.ValueOf(&test.EchoResponse{}),
						reflect.Zero(reflectutil.TypeError),
					},
				},
				expectAssertError: true,
			},
			"wrong status code": {
				expect: &Expect{
					Status: ExpectStatus{
						Code: "Invalid Argument",
					},
				},
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.Error(codes.NotFound, "not found")),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.Error(codes.NotFound, "not found")),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.FromProto(&spb.Status{
							Code:    int32(codes.InvalidArgument),
							Message: "invalid argument",
							Details: []*anypb.Any{
								mustAny(t,
									&errdetails.LocalizedMessage{
										Locale:  "ja-JP",
										Message: "エラー",
									},
								),
								mustAny(t,
									&errdetails.DebugInfo{
										Detail: "debug",
									},
								),
							},
						}).Err()),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.FromProto(&spb.Status{
							Code:    int32(codes.InvalidArgument),
							Message: "invalid argument",
							Details: []*anypb.Any{
								mustAny(t,
									&errdetails.LocalizedMessage{
										Locale:  "ja-JP",
										Message: "エラー",
									},
								),
								mustAny(t,
									&errdetails.DebugInfo{
										Detail: "debug",
									},
								),
							},
						}).Err()),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.FromProto(&spb.Status{
							Code:    int32(codes.InvalidArgument),
							Message: "invalid argument",
							Details: []*anypb.Any{
								mustAny(t,
									&errdetails.LocalizedMessage{
										Locale:  "ja-JP",
										Message: "エラー",
									},
								),
								mustAny(t,
									&errdetails.DebugInfo{
										Detail: "debug",
									},
								),
							},
						}).Err()),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.FromProto(&spb.Status{
							Code:    int32(codes.InvalidArgument),
							Message: "invalid argument",
							Details: []*anypb.Any{
								mustAny(t,
									&errdetails.LocalizedMessage{
										Locale:  "ja-JP",
										Message: "エラー",
									},
								),
								mustAny(t,
									&errdetails.DebugInfo{
										Detail: "debug",
									},
								),
							},
						}).Err()),
					},
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
				v: response{
					rvalues: []reflect.Value{
						reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
						reflect.ValueOf(status.FromProto(&spb.Status{
							Code:    int32(codes.InvalidArgument),
							Message: "invalid argument",
							Details: []*anypb.Any{
								mustAny(t,
									&errdetails.LocalizedMessage{
										Locale:  "ja-JP",
										Message: "エラー",
									},
								),
								mustAny(t,
									&errdetails.DebugInfo{
										Detail: "debug",
									},
								),
							},
						}).Err()),
					},
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
			"invalid type for rvalues of response": {
				expect: &Expect{},
				v: response{
					rvalues: []reflect.Value{},
				},
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

func mustAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
