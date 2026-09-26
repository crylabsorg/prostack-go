package ps_otel

import (
	"context"
	"net/http"
	"time"

	"github.com/crylabsorg/prostack-go/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/zap"
)

func InitMetrics(ctx context.Context, mux *http.ServeMux) func() {
	autoexport.RegisterMetricReader("ps_prometheus", func(context.Context) (metric.Reader, error) {
		registry := prometheus.NewRegistry()

		reader, err := otelprom.New(otelprom.WithRegisterer(registry))
		if err != nil {
			return nil, err
		}

		mux.Handle(
			"GET /otel-metrics",
			promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
		)
		return reader, nil
	})

	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to init metrics reader")
	}

	mp := metric.NewMeterProvider(metric.WithReader(reader))

	otel.SetMeterProvider(mp)

	if err := runtime.Start(); err != nil {
		logger.GetLogger(ctx).Fatal("failed to start runtime instrumentation", zap.Error(err))
	}

	shutdown := func() {
		const timeout = 10 * time.Second
		shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer shutdownCancel()
		if shutdownErr := mp.Shutdown(shutdownCtx); shutdownErr != nil {
			logger.GetLogger(ctx).Error("failed to shutdown metrics provider", zap.Error(shutdownErr))
		}
	}

	return shutdown
}
