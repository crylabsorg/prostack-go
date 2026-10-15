package ps_kafka //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestRecordCarrierPropagatesLowercaseHeaders(t *testing.T) {
	t.Parallel()
	record := &kgo.Record{Headers: []kgo.RecordHeader{
		{Key: "traceparent", Value: []byte("00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")},
		{Key: "tracestate", Value: []byte("vendor=value")},
		{Key: "baggage", Value: []byte("tenant=example")},
	}}
	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	ctx := propagator.Extract(context.Background(), recordCarrier{record})
	publisherContext := trace.SpanContextFromContext(ctx)
	require.True(t, publisherContext.IsValid())
	require.True(t, publisherContext.IsRemote())
	require.True(t, publisherContext.IsSampled())
	require.Equal(t, "0123456789abcdef0123456789abcdef", publisherContext.TraceID().String())
	require.Equal(t, "0123456789abcdef", publisherContext.SpanID().String())
	require.Equal(t, "vendor=value", publisherContext.TraceState().String())
	require.Equal(t, "example", baggage.FromContext(ctx).Member("tenant").Value())

	injected := &kgo.Record{}
	propagator.Inject(ctx, recordCarrier{injected})
	require.ElementsMatch(t, record.Headers, injected.Headers)
	require.ElementsMatch(t, []string{"traceparent", "tracestate", "baggage"}, (recordCarrier{injected}).Keys())
}

//nolint:paralleltest // The test temporarily replaces OpenTelemetry globals.
func TestProcessRecordTracing(t *testing.T) {
	recorder := kafkaSpanRecorder(t)
	handleErr, commitErr := errors.New("handle failed"), errors.New("commit failed")
	for _, testCase := range []struct {
		name      string
		handleErr error
		commitErr error
		cancel    bool
	}{
		{name: "success"},
		{name: "handler failure", handleErr: handleErr},
		{name: "commit failure", commitErr: commitErr},
		{name: "cancellation", handleErr: context.Canceled, cancel: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if testCase.cancel {
				cancel()
			}
			record := &kgo.Record{
				Topic: "user.created", Key: []byte("event-1"), Partition: 2, Offset: 42,
				Headers: []kgo.RecordHeader{
					{Key: "traceparent", Value: []byte("00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")},
					{Key: "tracestate", Value: []byte("vendor=value")},
					{Key: "baggage", Value: []byte("tenant=example")},
				},
			}
			committed := false
			var handlerSpanContext trace.SpanContext
			spanCount := len(recorder.Ended())
			err := processRecord(
				ctx,
				record,
				"email-sender",
				func(handleCtx context.Context, handled *kgo.Record) error {
					require.Same(t, record, handled)
					require.Equal(t, ctx.Err(), handleCtx.Err())
					require.Equal(t, "example", baggage.FromContext(handleCtx).Member("tenant").Value())
					handlerSpanContext = trace.SpanContextFromContext(handleCtx)
					return testCase.handleErr
				},
				func(commitCtx context.Context, committedRecord *kgo.Record) error {
					committed = true
					require.Same(t, record, committedRecord)
					require.Equal(t, handlerSpanContext, trace.SpanContextFromContext(commitCtx))
					return testCase.commitErr
				},
			)
			require.Equal(t, testCase.handleErr == nil, committed)
			expectedErr := testCase.handleErr
			if expectedErr == nil {
				expectedErr = testCase.commitErr
			}
			if expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, expectedErr)
			}
			spans := recorder.Ended()[spanCount:]
			require.Len(t, spans, 1)
			span := spans[0]
			require.Equal(t, "process user.created", span.Name())
			require.Equal(t, trace.SpanKindConsumer, span.SpanKind())
			require.False(t, span.Parent().IsValid())
			require.Equal(t, handlerSpanContext, span.SpanContext())
			require.Len(t, span.Links(), 1)
			publisherContext := span.Links()[0].SpanContext
			require.True(t, publisherContext.IsRemote())
			require.Equal(t, "0123456789abcdef0123456789abcdef", publisherContext.TraceID().String())
			require.Equal(t, "0123456789abcdef", publisherContext.SpanID().String())
			require.Equal(t, "vendor=value", publisherContext.TraceState().String())
			require.NotEqual(t, publisherContext.TraceID(), span.SpanContext().TraceID())
			expectedAttributes := []attribute.KeyValue{
				attribute.String("messaging.system", "kafka"),
				attribute.String("messaging.destination.name", "user.created"),
				attribute.String("messaging.message.id", "event-1"),
				attribute.String("messaging.destination.partition.id", "2"),
				attribute.Int64("messaging.kafka.offset", 42),
				attribute.String("messaging.consumer.group.name", "email-sender"),
				attribute.String("messaging.operation.name", "process"),
				attribute.String("messaging.operation.type", "process"),
			}
			if expectedErr == nil {
				require.Equal(t, codes.Unset, span.Status().Code)
				require.Empty(t, span.Events())
			} else {
				expectedAttributes = append(
					expectedAttributes,
					attribute.String("error.type", fmt.Sprintf("%T", expectedErr)),
				)
				require.Equal(t, codes.Error, span.Status().Code)
				require.Equal(t, expectedErr.Error(), span.Status().Description)
				require.Len(t, span.Events(), 1)
				require.Equal(t, "exception", span.Events()[0].Name)
				require.Contains(
					t,
					span.Events()[0].Attributes,
					attribute.String("exception.message", expectedErr.Error()),
				)
			}
			require.ElementsMatch(t, expectedAttributes, span.Attributes())
		})
	}
}

//nolint:paralleltest // The test temporarily replaces OpenTelemetry globals.
func TestPublishTracingOnCancellation(t *testing.T) {
	recorder := kafkaSpanRecorder(t)
	publisher, err := NewPublisher("127.0.0.1:1")
	require.NoError(t, err)
	t.Cleanup(publisher.Close)
	requestCtx, request := otel.Tracer("test").Start(context.Background(), "request")
	ctx, cancel := context.WithCancel(requestCtx)
	cancel()
	require.ErrorIs(t, publisher.Publish(ctx, kafkaTestMessage{}), context.Canceled)
	request.End()

	spans := recorder.Ended()
	require.Len(t, spans, 2)
	span := spans[0]
	require.Equal(t, "publish user.created", span.Name())
	require.Equal(t, trace.SpanKindProducer, span.SpanKind())
	require.Equal(t, request.SpanContext(), span.Parent())
	require.Equal(t, codes.Error, span.Status().Code)
	require.ElementsMatch(t, []attribute.KeyValue{
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.destination.name", "user.created"),
		attribute.String("messaging.message.id", "event-1"),
		attribute.String("messaging.operation.name", "publish"),
		attribute.String("messaging.operation.type", "send"),
		attribute.String("error.type", fmt.Sprintf("%T", context.Canceled)),
	}, span.Attributes())
	require.Len(t, span.Events(), 1)
	require.Equal(t, "exception", span.Events()[0].Name)
	require.Contains(t, span.Events()[0].Attributes, attribute.String("exception.message", context.Canceled.Error()))
}

func kafkaSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
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
	return recorder
}

type kafkaTestMessage struct{}

func (kafkaTestMessage) ID() string                      { return "event-1" }
func (kafkaTestMessage) Subject() string                 { return "user.created" }
func (kafkaTestMessage) Payload() []byte                 { return []byte(`{"id":"user-id"}`) }
func (kafkaTestMessage) TraceContext() map[string]string { return nil }
