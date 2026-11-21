package ps_kafka //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

//nolint:paralleltest // The test temporarily replaces the global tracer provider.
func TestProcessRecordTracing(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); require.NoError(t, provider.Shutdown(context.Background())) })
	expected := errors.New("commit failed")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := processRecord(ctx, &kgo.Record{Topic: "events"}, "group", func(ctx context.Context, _ *kgo.Record) error {
		require.True(t, trace.SpanFromContext(ctx).SpanContext().IsValid())
		require.NoError(t, ctx.Err())
		return nil
	}, func(ctx context.Context, _ *kgo.Record) error { return expected })
	require.ErrorIs(t, err, expected)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, codes.Error, spans[0].Status().Code)
}

//nolint:paralleltest // The test temporarily replaces OpenTelemetry globals.
func TestProcessRecordLinksPublisher(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	publisherCtx, publisher := provider.Tracer("test").Start(context.Background(), "publish")
	record := &kgo.Record{Topic: "events"}
	otel.GetTextMapPropagator().Inject(publisherCtx, kotel.NewRecordCarrier(record))
	publisher.End()
	err := processRecord(context.Background(), record, "group", func(context.Context, *kgo.Record) error { return nil }, func(context.Context, *kgo.Record) error { return nil })
	require.NoError(t, err)
	spans := recorder.Ended()
	require.Len(t, spans, 2)
	require.False(t, spans[1].Parent().IsValid())
	require.Len(t, spans[1].Links(), 1)
	require.Equal(t, publisher.SpanContext(), spans[1].Links()[0].SpanContext.WithRemote(false))
	require.NotEqual(t, publisher.SpanContext().TraceID(), spans[1].SpanContext().TraceID())
}
