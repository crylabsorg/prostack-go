package logger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func GetLogger(ctx context.Context) *zap.Logger {
	logger := zap.L()
	if ctx == nil {
		return logger
	}

	return logger.With(zap.Any("context", ctx))
}

// traceCore converts the context field to trace IDs only for the stdout core.
// The OTel core receives the original context and extracts trace information itself.
type traceCore struct{ zapcore.Core }

func (c traceCore) With(fields []zapcore.Field) zapcore.Core {
	return traceCore{c.Core.With(traceFields(fields))}
}

func traceFields(fields []zapcore.Field) []zapcore.Field {
	const traceFieldCount = 2
	result := make([]zapcore.Field, 0, len(fields)+traceFieldCount)
	for _, field := range fields {
		ctx, ok := field.Interface.(context.Context)
		if !ok {
			result = append(result, field)
			continue
		}

		spanContext := trace.SpanFromContext(ctx).SpanContext()
		if spanContext.IsValid() {
			result = append(result,
				zap.String("traceId", spanContext.TraceID().String()),
				zap.String("spanId", spanContext.SpanID().String()),
			)
		}
	}
	return result
}
