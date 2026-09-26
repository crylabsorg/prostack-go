//nolint:nolintlint,revive // ignore
package ps_grpc

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

func NewServer(healthServer *health.Server) *grpc.Server {
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

func Start(ctx context.Context, address string, grpcServer *grpc.Server) {
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to listen for gRPC", zap.String("address", address), zap.Error(err))
	}
	defer listener.Close()

	logger.GetLogger(ctx).Info("running grpc server", zap.String("address", address))

	errCh := make(chan error, 1)
	go func() { errCh <- grpcServer.Serve(listener) }()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			logger.GetLogger(ctx).Fatal("failed to serve gRPC", zap.Error(err))
		}
		return
	case <-ctx.Done():
	}
	logger.GetLogger(ctx).Info("start shutdown grpc server")

	grpcServer.GracefulStop()
	if err := <-errCh; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		logger.GetLogger(ctx).Fatal("failed to stop gRPC", zap.Error(err))
	}
	logger.GetLogger(ctx).Info("grpc server gracefully stopped")
}
