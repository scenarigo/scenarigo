package mockutil

import (
	"fmt"
	"testing"

	"github.com/scenarigo/scenarigo/testdata/gen/pb/test"
	"google.golang.org/protobuf/proto"
)

func TestProtoMessage_Matches(t *testing.T) {
	tests := map[string]struct {
		msg   proto.Message
		v     interface{}
		match bool
	}{
		"match": {
			msg: &test.EchoRequest{
				MessageId:   "xxx",
				MessageBody: "hello",
			},
			v: &test.EchoRequest{
				MessageId:   "xxx",
				MessageBody: "hello",
			},
			match: true,
		},
		"not match": {
			msg: &test.EchoRequest{
				MessageId:   "xxx",
				MessageBody: "hello",
			},
			v: &test.EchoRequest{
				MessageId:   "xxx",
				MessageBody: "bye",
			},
			match: false,
		},
		"different type": {
			msg:   &test.EchoRequest{},
			v:     &test.EchoResponse{},
			match: false,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			m := ProtoMessage(test.msg)
			if expect, got := test.match, m.Matches(test.v); expect != got {
				t.Errorf("expect %t but got %t", expect, got)
			}
		})
	}
}

func TestProtoMessage_String(t *testing.T) {
	req := &test.EchoRequest{
		MessageId:   "xxx",
		MessageBody: "hello",
	}
	m := ProtoMessage(req)
	if expect, got := fmt.Sprintf("is equal to %v", req), m.String(); expect != got {
		t.Errorf("expect %q but got %q", expect, got)
	}
}
