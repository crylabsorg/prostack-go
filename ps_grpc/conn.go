package ps_grpc

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	otelgrpc "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	_ "google.golang.org/grpc/health" // REQUIRED to enable health checks
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/test/bufconn"
)

func NewClientConn(ctx context.Context, address string, extra ...grpc.DialOption) (*grpc.ClientConn, error) {
	return newClientConn(ctx, address, extra...)
}

func NewClientConnWithBufconn(t *testing.T, ctx context.Context, listener *bufconn.Listener) *grpc.ClientConn {
	conn, err := NewClientConnWithBufconnContext(ctx, listener)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})
	return conn
}

func NewClientConnWithBufconnContext(ctx context.Context, listener *bufconn.Listener) (*grpc.ClientConn, error) {
	return newClientConn(
		ctx,
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
	)
}

func newClientConn(_ context.Context, address string, extra ...grpc.DialOption) (*grpc.ClientConn, error) {
	const (
		xtime             = 5 * time.Second
		timeout           = 2 * time.Second
		baseDelay         = 100 * time.Millisecond
		multiplier        = 1.6
		jitter            = 0.2
		maxDelay          = 2 * time.Second
		minConnectTimeout = 2 * time.Second
	)

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		// Keepalive detects connection failures more quickly.
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                xtime,   // Send a ping every 5 seconds.
			Timeout:             timeout, // Timeout for the ping response.
			PermitWithoutStream: true,    // Send pings even when no RPCs are active.
		}),

		// Aggressive backoff reconnects more quickly.
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  baseDelay,
				Multiplier: multiplier,
				Jitter:     jitter,
				MaxDelay:   maxDelay,
			},
			MinConnectTimeout: minConnectTimeout,
		}),
	}

	dialOpts = append(dialOpts, extra...)

	if strings.HasPrefix(address, "dns:///") {
		const serviceConfig = `{"loadBalancingConfig":[{"round_robin":{}}],"healthCheckConfig":{"serviceName":""}}`
		dialOpts = append(dialOpts, grpc.WithDefaultServiceConfig(serviceConfig))
	}

	client, err := grpc.NewClient(address, dialOpts...)
	if err != nil {
		return nil, err
	}

	client.Connect()

	return client, nil
}
