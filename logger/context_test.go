package logger

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/embedded"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type captureProvider struct {
	embedded.LoggerProvider
	logger *captureLogger
}

func (p captureProvider) Logger(string, ...log.LoggerOption) log.Logger { return p.logger }

type captureLogger struct {
	embedded.Logger
	ctx    context.Context
	record log.Record
}

func (*captureLogger) Enabled(context.Context, log.EnabledParameters) bool { return true }

func (l *captureLogger) Emit(ctx context.Context, record log.Record) {
	l.ctx = ctx
	l.record = record
}

func TestGetLoggerRoutesTraceContext(t *testing.T) {
	var stdout bytes.Buffer
	stdoutCore := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&stdout), zapcore.DebugLevel)
	otelLogger := &captureLogger{}
	log := zap.New(zapcore.NewTee(traceCore{stdoutCore}, otelzap.NewCore("test", otelzap.WithLoggerProvider(captureProvider{logger: otelLogger}))))
	previous := zap.L()
	zap.ReplaceGlobals(log)
	t.Cleanup(func() { zap.ReplaceGlobals(previous) })

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{2},
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)
	GetLogger(ctx).Info("hello", zap.String("key", "value"))

	require.Same(t, ctx, otelLogger.ctx)
	var stdoutRecord map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &stdoutRecord))
	require.Equal(t, spanContext.TraceID().String(), stdoutRecord["traceId"])
	require.Equal(t, spanContext.SpanID().String(), stdoutRecord["spanId"])
	require.Equal(t, "value", stdoutRecord["key"])
	require.NotContains(t, stdoutRecord, "context")
	var keys []string
	otelLogger.record.WalkAttributes(func(kv attribute.KeyValue) bool {
		keys = append(keys, string(kv.Key))
		return true
	})
	require.Equal(t, []string{"key"}, keys)
}
