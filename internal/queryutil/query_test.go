package queryutil

import (
	"context"
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/protocol/grpc/proto"
	"github.com/zoncoen/query-go"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestKeyExtractor_ExtractByKey(t *testing.T) {
	comp := proto.NewCompiler([]string{})
	fds, err := comp.Compile(context.Background(), []string{"testdata/test.proto"})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := fds.ResolveService("scenarigo.testdata.test.Test")
	if err != nil {
		t.Fatal(err)
	}
	method := svc.Methods().ByName("Echo")
	if method == nil {
		t.Fatal("failed to get method")
	}
	msg := dynamicpb.NewMessage(method.Input())
	msg.Set(method.Input().Fields().ByName("message_id"), protoreflect.ValueOf("1"))

	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			key    string
			opts   []query.Option
			expect any
		}{
			"name": {
				key:    "message_id",
				expect: "1",
			},
			"name (case insensitive)": {
				key:    "MESSAGE_ID",
				opts:   []query.Option{query.CaseInsensitive()},
				expect: "1",
			},
			"json": {
				key:    "messageId",
				expect: "1",
			},
			"json (case insensitive)": {
				key:    "messageid",
				opts:   []query.Option{query.CaseInsensitive()},
				expect: "1",
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				v, err := New(test.opts...).Key(test.key).Extract(msg)
				if err != nil {
					t.Fatal(err)
				}
				if got, expect := v, test.expect; got != expect {
					t.Errorf("expect %v but got %v", expect, got)
				}
			})
		}
	})

	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			key    string
			opts   []query.Option
			expect string
		}{
			"not found": {
				key:    "messageid",
				expect: `".messageid" not found`,
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				_, err := New(test.opts...).Key(test.key).Extract(msg)
				if err == nil {
					t.Fatal("no error")
				}
				if got, expect := err.Error(), test.expect; !strings.Contains(got, test.expect) {
					t.Errorf("expect %q but got %q", expect, got)
				}
			})
		}
	})
}
