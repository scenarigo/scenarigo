package mockutil

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

// ProtoMessage returns a matcher to compare protobuf messages.
func ProtoMessage(msg proto.Message) gomock.Matcher {
	return &protoMessageMatcher{
		message: msg,
	}
}

type protoMessageMatcher struct {
	message proto.Message
}

// Matches implements gomock.Matcher interface.
func (p *protoMessageMatcher) Matches(v any) bool {
	return cmp.Equal(v, p.message, protocmp.Transform())
}

// String implements gomock.Matcher interface.
func (p *protoMessageMatcher) String() string {
	return fmt.Sprintf("is equal to %v", p.message)
}
