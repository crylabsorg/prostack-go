package ps_otel //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
)

func InitTraces(ctx context.Context, _ string) func() {
	exporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to create trace exporter", zap.Error(err))
	}

	tp := trace.NewTracerProvider(trace.WithBatcher(exporter))

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	//nolint:reassign // Bootstrap instruments HTTP clients that use the process default transport.
	http.DefaultTransport = otelhttp.NewTransport(nil)

	shutdown := func() {
		const timeout = 10 * time.Second
		shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer shutdownCancel()
		if shutdownErr := tp.Shutdown(shutdownCtx); shutdownErr != nil {
			logger.GetLogger(ctx).Error("failed to shutdown tracer provider", zap.Error(shutdownErr))
		}
	}

	return shutdown
}
