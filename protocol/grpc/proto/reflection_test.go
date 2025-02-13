package proto

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/google/go-cmp/cmp"

	"github.com/scenarigo/scenarigo/internal/testutil"
)

func TestReflectionClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := startServer(t)
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}
	defer cc.Close()

	client := NewReflectionClient(ctx, cc)
	names, err := client.ListServices()
	if err != nil {
		t.Fatalf("failed to get services: %s", err)
	}
	if diff := cmp.Diff([]protoreflect.FullName{
		"grpc.reflection.v1.ServerReflection",
		"grpc.reflection.v1alpha.ServerReflection",
		"scenarigo.testdata.test.Test",
	}, names); diff != "" {
		t.Fatalf("request differs (-want +got):\n%s", diff)
	}

	if _, err := client.ResolveService(protoreflect.FullName("scenarigo.testdata.test.Test")); err != nil {
		t.Fatalf("failed to get service: %s", err)
	}
}

func startServer(t *testing.T) string {
	t.Helper()
	return testutil.StartTestGRPCServer(t, nil, testutil.EnableReflection())
}

func TestIsUnimplementedReflectionServiceError(t *testing.T) {
	tests := map[string]struct {
		err    error
		expect bool
	}{
		"true": {
			err:    status.New(codes.Unimplemented, unimplementedReflectionServiceMessage).Err(),
			expect: true,
		},
		"nil error": {
			expect: false,
		},
		"not status error": {
			err:    errors.New(unimplementedReflectionServiceMessage),
			expect: false,
		},
		"not unimplemented error": {
			err:    status.New(codes.Unknown, unimplementedReflectionServiceMessage).Err(),
			expect: false,
		},
		"not reflection service": {
			err:    status.New(codes.Unimplemented, "unknown service scenarigo.test.ServerReflection").Err(),
			expect: false,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			got := IsUnimplementedReflectionServiceError(test.err)
			if got != test.expect {
				t.Errorf("expect %t but got %t", test.expect, got)
			}
		})
	}
}
