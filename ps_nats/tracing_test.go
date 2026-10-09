package ps_nats

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

type linkedOutboxMessage struct {
	testOutboxMessage
	carrier map[string]string
}

func (m linkedOutboxMessage) TraceContext() map[string]string { return m.carrier }

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
	js := &testJetStream{publish: func(ctx context.Context, msg *nats.Msg) (*jetstream.PubAck, error) {
		extracted := propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(msg.Header))
		require.Equal(t, trace.SpanContextFromContext(ctx).SpanID(), trace.SpanContextFromContext(extracted).SpanID())
		return &jetstream.PubAck{}, nil
	}}
	results := (&outboxPublisher{js: js}).publishMessages(context.Background(), messages)
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
