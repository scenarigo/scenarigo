package testutil

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/google/go-cmp/cmp"
	testpb "github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

func TestStartTestGRPCServer(t *testing.T) {
	srv := TestGRPCServerFunc(func(ctx context.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
		return &testpb.EchoResponse{
			MessageId:   req.GetMessageId(),
			MessageBody: req.GetMessageBody(),
		}, nil
	})
	target := StartTestGRPCServer(t, srv, EnableReflection())
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	client := testpb.NewTestClient(conn)
	resp, err := client.Echo(context.Background(), &testpb.EchoRequest{
		MessageId:   "1",
		MessageBody: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&testpb.EchoResponse{
		MessageId:   "1",
		MessageBody: "hello",
	}, resp, protocmp.Transform()); diff != "" {
		t.Errorf("differs: (-want +got)\n%s", diff)
	}
}
