package ps_redis //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
)

func New(ctx context.Context, addr string) *redis.Client {
	options, err := redis.ParseURL(addr)
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to parse redis URL", zap.Error(err))
	}
	client := redis.NewClient(options)
	if traceErr := redisotel.InstrumentTracing(client); traceErr != nil {
		logger.GetLogger(ctx).Fatal("failed to instrument redis tracing", zap.Error(traceErr))
	}
	if metricsErr := redisotel.InstrumentMetrics(client); metricsErr != nil {
		logger.GetLogger(ctx).Fatal("failed to instrument redis metrics", zap.Error(metricsErr))
	}

	if err = client.Ping(ctx).Err(); err != nil {
		logger.GetLogger(ctx).Fatal("failed to connect to redis", zap.Error(err))
	}

	return client
}
