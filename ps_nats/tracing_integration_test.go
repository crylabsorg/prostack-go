package ps_nats_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/crylabsorg/prostack-go/ps_nats"
	"github.com/crylabsorg/prostack-go/ps_test"
)

//nolint:paralleltest // The test temporarily replaces OpenTelemetry globals.
func TestNATSWireContextAndRetry(t *testing.T) {
	connection, err := ps_nats.Connect(ps_test.NewNATSContainer(t))
	require.NoError(t, err)
	t.Cleanup(connection.Close)
	js, err := jetstream.New(connection)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	message := publisherMessage{}
	require.NoError(t, ps_nats.EnsureStream(ctx, js, "tracing", message.Subject()))
	consumer, err := js.CreateConsumer(ctx, "tracing", jetstream.ConsumerConfig{
		FilterSubject: message.Subject(),
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	member, err := baggage.NewMember("correlation", "wire-test")
	require.NoError(t, err)
	requestBaggage, err := baggage.New(member)
	require.NoError(t, err)
	requestCtx, requestSpan := provider.Tracer("test").Start(baggage.ContextWithBaggage(ctx, requestBaggage), "request")
	require.NoError(t, ps_nats.NewPublisher(js).Publish(requestCtx, message))
	requestSpan.End()

	drainingCtx, drain := context.WithCancel(ctx)
	defer drain()
	handlerErr := errors.New("temporary delivery failure")
	var receivedHeaders, receivedBaggage []string
	err = ps_nats.RunConsumer(ctx, drainingCtx, consumer, 1, func(msgCtx context.Context, msg jetstream.Msg) error {
		receivedHeaders = append(receivedHeaders, msg.Headers().Get("traceparent"))
		receivedBaggage = append(receivedBaggage, baggage.FromContext(msgCtx).Member("correlation").Value())
		if len(receivedHeaders) == 1 {
			return errors.Join(handlerErr, msg.Nak())
		}
		ackErr := msg.DoubleAck(msgCtx)
		if ackErr == nil {
			drain()
		}
		return ackErr
	})
	require.NoError(t, err)
	require.NoError(t, ctx.Err(), "the retried message must finish before the timeout")
	require.Len(t, receivedHeaders, 2)
	require.NotEmpty(t, receivedHeaders[0], "traceparent must be lowercase on the wire")
	require.Equal(t, receivedHeaders[0], receivedHeaders[1])
	require.Equal(t, []string{"wire-test", "wire-test"}, receivedBaggage)

	var publisherContext trace.SpanContext
	for _, span := range recorder.Ended() {
		if span.SpanKind() == trace.SpanKindProducer {
			publisherContext = span.SpanContext()
		}
	}
	require.True(t, publisherContext.IsValid())
	var consumerContexts []trace.SpanContext
	failed := 0
	for _, span := range recorder.Ended() {
		if span.SpanKind() != trace.SpanKindConsumer {
			continue
		}
		consumerContexts = append(consumerContexts, span.SpanContext())
		require.False(t, span.Parent().IsValid())
		require.NotEqual(t, publisherContext.TraceID(), span.SpanContext().TraceID())
		require.Len(t, span.Links(), 1)
		require.Equal(t, publisherContext, span.Links()[0].SpanContext.WithRemote(false))
		if span.Status().Code == codes.Error {
			failed++
			require.NotEmpty(t, span.Events())
		} else {
			require.Equal(t, codes.Unset, span.Status().Code)
		}
	}
	require.Len(t, consumerContexts, 2)
	require.NotEqual(t, consumerContexts[0].TraceID(), consumerContexts[1].TraceID())
	require.Equal(t, 1, failed)
}
