package grpcserver

import (
	"context"
	"net"
	"testing"

	pb "obsidian/internal/grpc/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func setupBufConnServer(t *testing.T) (pb.ObsidianServiceClient, func()) {
	lis := bufconn.Listen(bufSize)
	baseServer := grpc.NewServer()
	srv := New(nil)
	srv.Register(baseServer)

	go func() {
		_ = baseServer.Serve(lis)
	}()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufconn: %v", err)
	}

	client := pb.NewObsidianServiceClient(conn)
	cleanup := func() {
		_ = conn.Close()
		baseServer.Stop()
	}

	return client, cleanup
}

func TestValidation(t *testing.T) {
	client, cleanup := setupBufConnServer(t)
	defer cleanup()

	ctx := context.Background()

	// EnqueueJob validation
	_, err := client.EnqueueJob(ctx, &pb.EnqueueJobRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}

	// GetJobStatus validation
	_, err = client.GetJobStatus(ctx, &pb.GetJobStatusRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}

	// CancelJob validation
	_, err = client.CancelJob(ctx, &pb.CancelJobRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}
}
