package ps_redis

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
	if err := redisotel.InstrumentTracing(client); err != nil {
		logger.GetLogger(ctx).Fatal("failed to instrument redis tracing", zap.Error(err))
	}
	if err := redisotel.InstrumentMetrics(client); err != nil {
		logger.GetLogger(ctx).Fatal("failed to instrument redis metrics", zap.Error(err))
	}

	if err = client.Ping(ctx).Err(); err != nil {
		logger.GetLogger(ctx).Fatal("failed to connect to redis", zap.Error(err))
	}

	return client
}
