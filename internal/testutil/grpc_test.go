package testutil

import (
	"context"
	"errors"
	"io"
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

func TestServerStreamEcho(t *testing.T) {
	srv := TestGRPCServerFunc(func(_ context.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
		return &testpb.EchoResponse{
			MessageId:   req.GetMessageId(),
			MessageBody: req.GetMessageBody(),
		}, nil
	})
	target := StartTestGRPCServer(t, srv)
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	client := testpb.NewTestClient(conn)
	stream, err := client.ServerStreamEcho(context.Background(), &testpb.EchoRequest{
		MessageId:   "1",
		MessageBody: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []*testpb.EchoResponse
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, resp)
	}
	want := []*testpb.EchoResponse{
		{MessageId: "1", MessageBody: "hello-0"},
		{MessageId: "1", MessageBody: "hello-1"},
		{MessageId: "1", MessageBody: "hello-2"},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("differs: (-want +got)\n%s", diff)
	}
}

func TestClientStreamEcho(t *testing.T) {
	srv := TestGRPCServerFunc(func(_ context.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
		return &testpb.EchoResponse{
			MessageId:   req.GetMessageId(),
			MessageBody: req.GetMessageBody(),
		}, nil
	})
	target := StartTestGRPCServer(t, srv)
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	client := testpb.NewTestClient(conn)
	stream, err := client.ClientStreamEcho(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"hello", "world"} {
		if err := stream.Send(&testpb.EchoRequest{
			MessageId:   "1",
			MessageBody: body,
		}); err != nil {
			t.Fatal(err)
		}
	}
	resp, err := stream.CloseAndRecv()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&testpb.EchoResponse{
		MessageId:   "aggregated",
		MessageBody: "hello,world",
	}, resp, protocmp.Transform()); diff != "" {
		t.Errorf("differs: (-want +got)\n%s", diff)
	}
}

func TestBidiStreamEcho(t *testing.T) {
	srv := TestGRPCServerFunc(func(_ context.Context, req *testpb.EchoRequest) (*testpb.EchoResponse, error) {
		return &testpb.EchoResponse{
			MessageId:   req.GetMessageId(),
			MessageBody: req.GetMessageBody(),
		}, nil
	})
	target := StartTestGRPCServer(t, srv)
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	client := testpb.NewTestClient(conn)
	stream, err := client.BidiStreamEcho(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"hello", "world"} {
		if err := stream.Send(&testpb.EchoRequest{
			MessageId:   "1",
			MessageBody: body,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	var got []*testpb.EchoResponse
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, resp)
	}
	want := []*testpb.EchoResponse{
		{MessageId: "1", MessageBody: "re: hello"},
		{MessageId: "1", MessageBody: "re: world"},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("differs: (-want +got)\n%s", diff)
	}
}
