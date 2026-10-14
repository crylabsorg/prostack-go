package ps_grpc //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"net"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
)

type blockingHealthServer struct {
	healthgrpc.UnimplementedHealthServer

	started chan struct{}
	finish  chan struct{}
	result  chan error
}

func (s *blockingHealthServer) Check(ctx context.Context,
	_ *healthgrpc.HealthCheckRequest,
) (*healthgrpc.HealthCheckResponse, error) {
	close(s.started)
	select {
	case <-s.finish:
	case <-ctx.Done():
	}
	s.result <- ctx.Err()
	return &healthgrpc.HealthCheckResponse{Status: healthgrpc.HealthCheckResponse_SERVING}, ctx.Err()
}

func TestRunWaitsForRPCAndCanForceCancellation(t *testing.T) {
	t.Parallel()
	for _, force := range []bool{false, true} {
		name := "graceful"
		if force {
			name = "forced"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				const bufferSize = 1024 * 1024
				listener := bufconn.Listen(bufferSize)
				defer listener.Close()
				server := grpc.NewServer()
				defer server.Stop()
				handler := &blockingHealthServer{
					started: make(chan struct{}), finish: make(chan struct{}), result: make(chan error, 1),
				}
				healthgrpc.RegisterHealthServer(server, handler)
				stopAccepting := make(chan struct{})
				runResult := make(chan error, 1)
				go func() { runResult <- serve(ctx, stopAccepting, server, listener) }()
				connection, err := grpc.NewClient("passthrough:///shutdown-test",
					grpc.WithTransportCredentials(insecure.NewCredentials()),
					grpc.WithContextDialer(func(dialCtx context.Context, _ string) (net.Conn, error) {
						return listener.DialContext(dialCtx)
					}),
				)
				require.NoError(t, err)
				defer connection.Close()
				rpcResult := make(chan error, 1)
				go func() {
					_, rpcErr := healthgrpc.NewHealthClient(connection).Check(
						t.Context(), &healthgrpc.HealthCheckRequest{},
					)
					rpcResult <- rpcErr
				}()
				<-handler.started
				close(stopAccepting)
				synctest.Wait()
				require.NoError(t, ctx.Err(), "graceful shutdown must preserve the work context")
				select {
				case <-runResult:
					require.FailNow(t, "Run returned while the RPC was still active")
				default:
				}
				if force {
					cancel()
					require.Error(t, <-rpcResult)
					require.ErrorIs(t, <-handler.result, context.Canceled)
				} else {
					close(handler.finish)
					require.NoError(t, <-rpcResult)
					require.NoError(t, <-handler.result)
				}
				require.NoError(t, <-runResult)
			})
		})
	}
}
