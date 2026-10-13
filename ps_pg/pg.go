package ps_pg //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
)

func New(ctx context.Context, url string) (*pgxpool.Pool, func()) {
	pool, disconnect, err := Connect(ctx, url)
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to connect to PostgreSQL", zap.Error(err))
	}
	logger.GetLogger(ctx).Debug("connected to PostgreSQL")
	return pool, disconnect
}

func Connect(ctx context.Context, url string) (*pgxpool.Pool, func(), error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, nil, err
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	if statsErr := otelpgx.RecordStats(pool); statsErr != nil {
		pool.Close()
		return nil, nil, statsErr
	}

	const pingTimeout = 5 * time.Second
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if pingErr := pool.Ping(pingCtx); pingErr != nil {
		pool.Close()
		return nil, nil, pingErr
	}
	return pool, pool.Close, nil
}
