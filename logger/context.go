package logger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func GetLogger(ctx context.Context) *zap.Logger {
	//nolint:forbidigo // fine in this package
	logger := zap.L()
	if ctx == nil {
		return logger
	}

	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return logger
	}

	return logger.With(
		zap.String("traceId", span.SpanContext().TraceID().String()),
		zap.String("spanId", span.SpanContext().SpanID().String()),
	)
}
