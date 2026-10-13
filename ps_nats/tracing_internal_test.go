package ps_nats //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type tracingMessage struct {
	jetstream.Msg

	headers nats.Header
}

func (m tracingMessage) Headers() nats.Header { return m.headers }
func (m tracingMessage) Subject() string      { return "user.created" }

//nolint:paralleltest // The test and its subtests share OpenTelemetry globals and a span recorder.
func TestConsumerTraceLink(t *testing.T) {
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

	publisherCtx, publisher := provider.Tracer("test").Start(context.Background(), "publish")
	defer publisher.End()
	headers := nats.Header{}
	otel.GetTextMapPropagator().Inject(publisherCtx, propagation.HeaderCarrier(headers))
	for _, test := range []struct {
		name    string
		headers nats.Header
		links   int
	}{
		{"publisher context", headers, 1},
		{"no context", nil, 0},
		{"invalid context", nats.Header{"Traceparent": {"invalid"}}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			consumerCtx, span := (&consumer{}).traceContext(ctx, tracingMessage{headers: test.headers})
			span.End()
			cancel()
			require.ErrorIs(t, consumerCtx.Err(), context.Canceled)
			recorded := recorder.Ended()[len(recorder.Ended())-1]
			require.False(t, recorded.Parent().IsValid())
			require.NotEqual(t, publisher.SpanContext().TraceID(), recorded.SpanContext().TraceID())
			require.Equal(t, trace.SpanKindConsumer, recorded.SpanKind())
			require.Len(t, recorded.Links(), test.links)
			if test.links > 0 {
				require.Equal(t, publisher.SpanContext().TraceID(), recorded.Links()[0].SpanContext.TraceID())
				require.Equal(t, publisher.SpanContext().SpanID(), recorded.Links()[0].SpanContext.SpanID())
			}
		})
	}
}
