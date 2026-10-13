package ps_otel //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
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

		if shutdownErr := lp.Shutdown(shutdownCtx); shutdownErr != nil {
			logger.GetLogger(context.Background()).Error(
				"failed to shutdown logs provider",
				zap.Error(shutdownErr),
			)
		}
	}
}
