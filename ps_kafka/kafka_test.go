package ps_kafka_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/crylabsorg/prostack-go/ps_kafka"
	"github.com/crylabsorg/prostack-go/ps_test"
)

type testMessage struct{ id string }

func (m testMessage) ID() string                      { return m.id }
func (m testMessage) Subject() string                 { return "user.created" }
func (m testMessage) Payload() []byte                 { return []byte(`{"id":"user-id"}`) }
func (m testMessage) TraceContext() map[string]string { return nil }

//nolint:paralleltest // The test temporarily replaces OpenTelemetry globals.
func TestKafkaDeliveryRetryCommitAndTracing(t *testing.T) {
	brokers := ps_test.NewKafkaContainer(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	publisher, err := ps_kafka.NewPublisher(brokers...)
	require.NoError(t, err)
	defer publisher.Close()
	require.NoError(t, publisher.Ping(ctx))
	requestCtx, request := provider.Tracer("test").Start(ctx, "request")
	require.NoError(t, publisher.Publish(requestCtx, testMessage{"event-1"}))
	request.End()

	handleErr := errors.New("email provider unavailable")
	err = ps_kafka.RunConsumer(
		ctx,
		ctx,
		brokers,
		"user.created",
		"email-sender",
		func(_ context.Context, record *kgo.Record) error {
			require.Equal(t, "event-1", string(record.Key))
			require.JSONEq(t, `{"id":"user-id"}`, string(record.Value))
			return handleErr
		},
	)
	require.ErrorIs(t, err, handleErr)

	// The same group receives the failed record again; drain still permits commit.
	drainingCtx, drain := context.WithCancel(ctx)
	defer drain()
	err = ps_kafka.RunConsumer(
		ctx,
		drainingCtx,
		brokers,
		"user.created",
		"email-sender",
		func(handleCtx context.Context, record *kgo.Record) error {
			require.Equal(t, "event-1", string(record.Key))
			drain()
			require.NoError(t, handleCtx.Err())
			return nil
		},
	)
	require.NoError(t, err)
	require.NoError(t, publisher.Publish(ctx, testMessage{"event-2"}))
	drainingCtx, drainAgain := context.WithCancel(ctx)
	defer drainAgain()
	err = ps_kafka.RunConsumer(
		ctx,
		drainingCtx,
		brokers,
		"user.created",
		"email-sender",
		func(_ context.Context, record *kgo.Record) error {
			// A committed event must not be delivered on restart.
			require.Equal(t, "event-2", string(record.Key))
			drainAgain()
			return nil
		},
	)
	require.NoError(t, err)

	var producerContexts []trace.SpanContext
	consumerCount, failedCount := 0, 0
	for _, span := range recorder.Ended() {
		if span.SpanKind() == trace.SpanKindProducer {
			producerContexts = append(producerContexts, span.SpanContext())
		}
	}
	require.Len(t, producerContexts, 2)
	for _, span := range recorder.Ended() {
		if span.SpanKind() != trace.SpanKindConsumer {
			continue
		}
		consumerCount++
		require.False(t, span.Parent().IsValid())
		require.Len(t, span.Links(), 1)
		require.Contains(t, producerContexts, span.Links()[0].SpanContext.WithRemote(false))
		if span.Status().Code == codes.Error {
			failedCount++
			require.NotEmpty(t, span.Events())
		}
	}
	require.Equal(t, 3, consumerCount)
	require.Equal(t, 1, failedCount)
}
