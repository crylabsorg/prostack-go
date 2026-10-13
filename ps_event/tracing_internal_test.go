package ps_event //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type linkedOutboxMessage struct {
	testOutboxMessage

	carrier map[string]string
}

func (m linkedOutboxMessage) TraceContext() map[string]string { return m.carrier }

//nolint:paralleltest // The test temporarily replaces OpenTelemetry globals.
func TestOutboxPublicationLinksEachRequest(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	var messages []OutboxMessage
	sources := map[string]trace.SpanContext{}
	for _, id := range []string{"first", "second"} {
		ctx, span := provider.Tracer("test").Start(context.Background(), id)
		carrier := propagation.MapCarrier{}
		propagation.TraceContext{}.Inject(ctx, carrier)
		sources[id] = span.SpanContext()
		span.End()
		messages = append(messages, linkedOutboxMessage{testOutboxMessage{id}, carrier})
	}
	messages = append(messages, testOutboxMessage{"legacy"})
	publisher := testPublisher(func(ctx context.Context, _ OutboxMessage) error {
		headers := propagation.MapCarrier{}
		otel.GetTextMapPropagator().Inject(ctx, headers)
		extracted := propagation.TraceContext{}.Extract(context.Background(), headers)
		require.Equal(t, trace.SpanContextFromContext(ctx).SpanID(), trace.SpanContextFromContext(extracted).SpanID())
		return nil
	})
	results := (&outboxPublisher{publisher: publisher}).publishMessages(context.Background(), messages)
	for _, result := range results {
		require.NoError(t, result.Err)
	}
	publications := 0
	for _, span := range recorder.Ended() {
		if span.SpanKind() != trace.SpanKindProducer || span.Name() == "publish batch" {
			continue
		}
		publications++
		require.True(t, span.Parent().IsValid())
		if len(span.Links()) == 0 {
			continue
		}
		require.Len(t, span.Links(), 1)
		source := span.Links()[0].SpanContext
		require.NotEqual(t, source.TraceID(), span.SpanContext().TraceID())
		require.Contains(t, []trace.SpanContext{sources["first"], sources["second"]}, source.WithRemote(false))
	}
	require.Equal(t, 3, publications)
	linked := 0
	for _, span := range recorder.Ended() {
		if len(span.Links()) > 0 {
			linked++
		}
	}
	require.Equal(t, 2, linked)
}
