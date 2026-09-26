package ps_otel

import (
	"context"
	"time"

	"github.com/crylabsorg/prostack-go/logger"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
	"go.uber.org/zap"
)

func InitLogs(ctx context.Context) func() {
	exporter, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to create logs exporter", zap.Error(err))
	}

	bp := log.NewBatchProcessor(exporter)
	lp := log.NewLoggerProvider(log.WithProcessor(bp))
	global.SetLoggerProvider(lp)

	return func() {
		const timeout = 10 * time.Second
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()

		if err := lp.Shutdown(shutdownCtx); err != nil {
			logger.GetLogger(context.Background()).Error(
				"failed to shutdown logs provider",
				zap.Error(err),
			)
		}
	}
}
