package grpc

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/googleapis/rpc/errdetails"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/testdata/gen/pb/test"
	"github.com/zoncoen/yaml"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExpect_Build(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tests := map[string]struct {
			vars   interface{}
			expect *Expect
			v      []reflect.Value
		}{
			"default": {
				expect: &Expect{},
				v: []reflect.Value{
					reflect.ValueOf(&test.EchoResponse{}),
					reflect.Zero(reflect.TypeOf((*error)(nil)).Elem()),
				},
			},
			"code": {
				expect: &Expect{
					Code: strconv.Itoa(int(codes.InvalidArgument)),
				},
				v: []reflect.Value{
					reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
					reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
				},
			},
			"code string": {
				expect: &Expect{
					Code: "InvalidArgument",
				},
				v: []reflect.Value{
					reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
					reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
				},
			},
			"assert body": {
				expect: &Expect{
					Code: "OK",
					Body: yaml.MapSlice{
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
				v: []reflect.Value{
					reflect.ValueOf(&test.EchoResponse{
						MessageId:   "1",
						MessageBody: "hello",
					}),
					reflect.Zero(reflect.TypeOf((*error)(nil)).Elem()),
				},
			},
			"assert in case of error": {
				expect: &Expect{
					Status: ExpectStatus{
						Code:    "InvalidArgument",
						Message: "invalid argument",
						Details: []yaml.MapSlice{
							yaml.MapSlice{
								yaml.MapItem{
									Key: "google.rpc.LocalizedMessage",
									Value: yaml.MapSlice{
										yaml.MapItem{
											Key:   "locale",
											Value: "ja-JP",
										},
									},
								},
							},
							yaml.MapSlice{
								yaml.MapItem{
									Key: "google.rpc.DebugInfo",
									Value: yaml.MapSlice{
										yaml.MapItem{
											Key:   "detail",
											Value: "debug",
										},
									},
								},
							},
						},
					},
				},
				v: []reflect.Value{
					reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
					reflect.ValueOf(mustWithDetails(
						status.New(codes.InvalidArgument, "invalid argument"),
						&errdetails.LocalizedMessage{
							Locale:  "ja-JP",
							Message: "エラー",
						},
						&errdetails.DebugInfo{
							Detail: "debug",
						},
					).Err()),
				},
			},
			"with vars": {
				vars: map[string]string{"body": "hello"},
				expect: &Expect{
					Code: "OK",
					Body: yaml.MapSlice{
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
				v: []reflect.Value{
					reflect.ValueOf(&test.EchoResponse{
						MessageId:   "1",
						MessageBody: "hello",
					}),
					reflect.Zero(reflect.TypeOf((*error)(nil)).Elem()),
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
			v                 interface{}
			expectBuildError  bool
			expectAssertError bool
		}{
			"return value must be []reflect.Value": {
				expect:            &Expect{},
				v:                 struct{}{},
				expectAssertError: true,
			},
			"the length of return values must be 2": {
				expect: &Expect{},
				v: []reflect.Value{
					reflect.ValueOf(&test.EchoResponse{}),
				},
				expectAssertError: true,
			},
			"fist return value must be proto.Message": {
				expect: &Expect{},
				v: []reflect.Value{
					reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
					reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
				},
				expectAssertError: true,
			},
			"second return value must be error": {
				expect: &Expect{},
				v: []reflect.Value{
					reflect.ValueOf(&test.EchoResponse{}),
					reflect.ValueOf(&test.EchoResponse{}),
				},
				expectAssertError: true,
			},
			"wrong code": {
				expect: &Expect{
					Code: "OK",
				},
				v: []reflect.Value{
					reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
					reflect.ValueOf(status.New(codes.InvalidArgument, "invalid argument").Err()),
				},
				expectAssertError: true,
			},
			"wrong body": {
				expect: &Expect{
					Code: "OK",
					Body: yaml.MapSlice{
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
				v: []reflect.Value{
					reflect.ValueOf(&test.EchoResponse{
						MessageId:   "1",
						MessageBody: "hell",
					}),
					reflect.Zero(reflect.TypeOf((*error)(nil)).Elem()),
				},
				expectAssertError: true,
			},
			"failed to execute template": {
				expect: &Expect{
					Code: "OK",
					Body: yaml.MapSlice{
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
				v: []reflect.Value{
					reflect.ValueOf(&test.EchoResponse{
						MessageId:   "1",
						MessageBody: "hello",
					}),
					reflect.Zero(reflect.TypeOf((*error)(nil)).Elem()),
				},
				expectBuildError: true,
			},
			"wrong status code": {
				expect: &Expect{
					Status: ExpectStatus{
						Code: "Invalid Argument",
					},
				},
				v: []reflect.Value{
					reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
					reflect.ValueOf(status.Error(codes.NotFound, "not found")),
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
				v: []reflect.Value{
					reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
					reflect.ValueOf(status.Error(codes.NotFound, "not found")),
				},
				expectAssertError: true,
			},
			"wrong status details: name is wrong": {
				expect: &Expect{
					Status: ExpectStatus{
						Details: []yaml.MapSlice{
							yaml.MapSlice{
								yaml.MapItem{
									Key: "google.rpc.Invalid",
									Value: yaml.MapSlice{
										yaml.MapItem{
											Key:   "detail",
											Value: "debug",
										},
									},
								},
							},
						},
					},
				},
				v: []reflect.Value{
					reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
					reflect.ValueOf(mustWithDetails(
						status.New(codes.InvalidArgument, "invalid argument"),
						&errdetails.LocalizedMessage{
							Locale:  "ja-JP",
							Message: "エラー",
						},
						&errdetails.DebugInfo{
							Detail: "debug",
						},
					).Err()),
				},
				expectAssertError: true,
			},
			"wrong status details: value is wrong": {
				expect: &Expect{
					Status: ExpectStatus{
						Details: []yaml.MapSlice{
							yaml.MapSlice{
								yaml.MapItem{
									Key: "google.rpc.DebugInfo",
									Value: yaml.MapSlice{
										yaml.MapItem{
											Key:   "detail",
											Value: "unknown",
										},
									},
								},
							},
						},
					},
				},
				v: []reflect.Value{
					reflect.Zero(reflect.TypeOf(&test.EchoResponse{})),
					reflect.ValueOf(mustWithDetails(
						status.New(codes.InvalidArgument, "invalid argument"),
						&errdetails.LocalizedMessage{
							Locale:  "ja-JP",
							Message: "エラー",
						},
						&errdetails.DebugInfo{
							Detail: "debug",
						},
					).Err()),
				},
				expectAssertError: true,
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

				err = assertion.Assert(test.v)
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

func mustWithDetails(s *status.Status, details ...proto.Message) *status.Status {
	ss, err := s.WithDetails(details...)
	if err != nil {
		panic(err)
	}
	return ss
}
