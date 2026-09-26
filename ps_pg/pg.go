package ps_pg

import (
	"context"
	"time"

	"github.com/crylabsorg/prostack-go/logger"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func New(ctx context.Context, url string) (*pgxpool.Pool, func()) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to parse PostgreSQL URL", zap.Error(err))
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to create PostgreSQL pool", zap.Error(err))
	}
	if err := otelpgx.RecordStats(pool); err != nil {
		pool.Close()
		logger.GetLogger(ctx).Fatal("failed to instrument PostgreSQL metrics", zap.Error(err))
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		logger.GetLogger(ctx).Fatal("failed to connect to PostgreSQL", zap.Error(err))
	}
	logger.GetLogger(ctx).Debug("connected to PostgreSQL")

	return pool, pool.Close
}
