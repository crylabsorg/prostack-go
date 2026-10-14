//nolint:nolintlint,revive // ignore
package ps_grpc //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/crylabsorg/prostack-go/logger"

	otelgrpc "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

// Run serves until stopAccepting closes, then waits for active RPCs.
// Canceling ctx forces remaining RPCs to stop.
// register installs application services before serving starts.
// Run returns after serving and shutdown have finished.
func Run(
	ctx context.Context,
	stopAccepting <-chan struct{},
	address string,
	healthServer *health.Server,
	register func(*grpc.Server),
) error {
	server := newServer(healthServer)
	register(server)
	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(ctx, "tcp", address)
	if err != nil {
		return err
	}
	logger.GetLogger(ctx).Info("running grpc server", zap.String("address", listener.Addr().String()))
	return serve(ctx, stopAccepting, server, listener)
}

func serve(ctx context.Context, stopAccepting <-chan struct{}, server *grpc.Server, listener net.Listener) error {
	serveResult := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, grpc.ErrServerStopped) {
			err = nil
		}
		serveResult <- err
	}()

	select {
	case err := <-serveResult:
		server.Stop()
		return err
	case <-stopAccepting:
	case <-ctx.Done():
	}

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		server.GracefulStop()
	}()
	select {
	case <-stopped:
	case <-ctx.Done():
		server.Stop()
	}
	return <-serveResult
}

func newServer(healthServer *health.Server) *grpc.Server {
	const (
		maxConnectionIdle     = 15 * time.Second
		maxConnectionAge      = 60 * time.Second
		maxConnectionAgeGrace = 60 * time.Second
		xtime                 = 5 * time.Second
		timeout               = 2 * time.Second
		minTime               = 5 * time.Second
	)

	serverOptions := []grpc.ServerOption{
		grpc.Creds(insecure.NewCredentials()),
		grpc.ChainUnaryInterceptor(RecoveryInterceptor, ErrorInterceptor),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     maxConnectionIdle,     // Close idle connections.
			MaxConnectionAge:      maxConnectionAge,      // Force connections to be refreshed.
			MaxConnectionAgeGrace: maxConnectionAgeGrace, // Grace period for completing RPCs.
			Time:                  xtime,                 // Send a ping every 5 seconds.
			Timeout:               timeout,               // Timeout for the ping response.
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             minTime, // Minimum interval between client pings.
			PermitWithoutStream: true,    // Allow pings when no streams are active.
		}),
	}
	server := grpc.NewServer(serverOptions...)
	reflection.Register(server)
	healthgrpc.RegisterHealthServer(server, healthServer)
	return server
}
