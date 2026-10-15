package ps_nats //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
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
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	traceState, err := trace.ParseTraceState("vendor=value")
	require.NoError(t, err)
	parentCtx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{2},
		TraceState: traceState,
	}))
	publisherCtx, publisher := provider.Tracer("test").Start(parentCtx, "publish")
	defer publisher.End()
	headers := nats.Header{}
	propagation.TraceContext{}.Inject(publisherCtx, headerCarrier(headers))
	headers["baggage"] = []string{"customer=acme", "region=west"}
	for _, test := range []struct {
		name    string
		headers nats.Header
		links   int
	}{
		{"lowercase", headers, 1},
		{"legacy HTTP case", nats.Header{
			"Traceparent": headers["traceparent"],
			"Tracestate":  headers["tracestate"],
			"Baggage":     headers["baggage"],
		}, 1},
		{"mixed case", nats.Header{
			"tRaCePaReNt": headers["traceparent"],
			"tRaCeStAtE":  headers["tracestate"],
			"bAgGaGe":     headers["baggage"],
		}, 1},
		{"exact match wins", nats.Header{
			"traceparent": headers["traceparent"], "Traceparent": {"invalid"},
			"tracestate": headers["tracestate"], "Tracestate": {"other=ignored"},
			"baggage": headers["baggage"], "Baggage": {"customer=ignored"},
		}, 1},
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
				require.Equal(t, "vendor=value", recorded.Links()[0].SpanContext.TraceState().String())
				require.True(t, recorded.Links()[0].SpanContext.IsRemote())
				require.Equal(t, "acme", baggage.FromContext(consumerCtx).Member("customer").Value())
				require.Equal(t, "west", baggage.FromContext(consumerCtx).Member("region").Value())
			}
		})
	}
}

func TestHeaderCarrier(t *testing.T) {
	t.Parallel()
	headers := nats.Header{}
	carrier := headerCarrier(headers)
	carrier.Set("traceparent", "first")
	carrier.Set("traceparent", "second")
	require.Equal(t, nats.Header{"traceparent": {"second"}}, headers)
	require.Equal(t, "second", carrier.Get("Traceparent"))
	require.Equal(t, []string{"second"}, carrier.Values("TRACEPARENT"))
	require.ElementsMatch(t, []string{"traceparent"}, carrier.Keys())
	require.Empty(t, carrier.Get("missing"))
	require.Nil(t, carrier.Values("missing"))
}

type tracingConsumer struct {
	jetstream.Consumer

	iterator jetstream.MessagesContext
}

func (c tracingConsumer) Messages(...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
	return c.iterator, nil
}

type tracingIterator struct {
	jetstream.MessagesContext

	messages []jetstream.Msg
	stopped  bool
}

func (i *tracingIterator) Next(...jetstream.NextOpt) (jetstream.Msg, error) {
	if len(i.messages) == 0 {
		return nil, jetstream.ErrMsgIteratorClosed
	}
	msg := i.messages[0]
	i.messages = i.messages[1:]
	return msg, nil
}

func (i *tracingIterator) Stop() { i.stopped = true }

//nolint:paralleltest // The test and its subtests share OpenTelemetry globals and a span recorder.
func TestRunConsumerTracing(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	for _, test := range []struct {
		name      string
		handleErr error
		messageID string
	}{
		{name: "success"},
		{name: "handler failure", handleErr: errors.New("handler failed"), messageID: "event-id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := nats.Header{}
			if test.messageID != "" {
				headers.Set(nats.MsgIdHdr, test.messageID)
			}
			iterator := &tracingIterator{messages: []jetstream.Msg{tracingMessage{headers: headers}}}
			handlerSpans := make(chan trace.SpanContext, 1)
			endedBefore := len(recorder.Ended())
			require.NoError(t, RunConsumer(
				context.Background(), context.Background(), tracingConsumer{iterator: iterator}, 1,
				func(ctx context.Context, _ jetstream.Msg) error {
					handlerSpans <- trace.SpanContextFromContext(ctx)
					return test.handleErr
				},
			))
			require.True(t, iterator.stopped)
			require.Len(t, recorder.Ended(), endedBefore+1)
			span := recorder.Ended()[endedBefore]
			require.Equal(t, <-handlerSpans, span.SpanContext())
			require.Equal(t, "process user.created", span.Name())
			require.Equal(t, trace.SpanKindConsumer, span.SpanKind())
			attributes := attribute.NewSet(span.Attributes()...)
			for key, value := range map[attribute.Key]string{
				"messaging.system":           "nats",
				"messaging.destination.name": "user.created",
				"messaging.operation.name":   "process",
				"messaging.operation.type":   "process",
			} {
				actual, ok := attributes.Value(key)
				require.True(t, ok, "missing attribute %s", key)
				require.Equal(t, value, actual.AsString())
			}
			messageID, hasMessageID := attributes.Value("messaging.message.id")
			require.Equal(t, test.messageID != "", hasMessageID)
			if hasMessageID {
				require.Equal(t, test.messageID, messageID.AsString())
			}
			if test.handleErr != nil {
				require.Equal(t, codes.Error, span.Status().Code)
				require.Equal(t, test.handleErr.Error(), span.Status().Description)
				errorType, ok := attributes.Value("error.type")
				require.True(t, ok)
				require.Equal(t, fmt.Sprintf("%T", test.handleErr), errorType.AsString())
				require.Len(t, span.Events(), 1)
				require.Equal(t, "exception", span.Events()[0].Name)
				eventAttributes := attribute.NewSet(span.Events()[0].Attributes...)
				errorMessage, ok := eventAttributes.Value("exception.message")
				require.True(t, ok)
				require.Equal(t, test.handleErr.Error(), errorMessage.AsString())
			} else {
				require.Equal(t, codes.Unset, span.Status().Code)
				require.Empty(t, span.Events())
				require.False(t, attributes.HasValue("error.type"))
			}
		})
	}
}
