package ps_test

import (
	"context"
	"errors"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/crylabsorg/prostack-go/ps_grpc"
)

func NewTestGRPC(ctx context.Context, grpcServer *grpc.Server) (*grpc.ClientConn, func()) {
	const buffconnSz = 1024 * 1024
	listener := bufconn.Listen(buffconnSz)

	errCh := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- err
		}
		close(errCh)
	}()

	cleanup1 := func() {
		grpcServer.GracefulStop()
		for err := range errCh {
			_, _ = fmt.Fprintf(os.Stderr, "failed to stop grpc test server: %v\n", err)
			os.Exit(1)
		}
	}

	client, err := ps_grpc.NewClientConnWithBufconnContext(ctx, listener)
	if err != nil {
		cleanup1()
		_, _ = fmt.Fprintf(os.Stderr, "failed to initialize grpc test client: %v\n", err)
		os.Exit(1)
	}

	cleanup := func() {
		cleanup1()
		if err1 := client.Close(); err1 != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to close grpc test client: %v\n", err1)
			os.Exit(1)
		}
	}

	return client, cleanup
}
