package ps_nats_test

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

	"github.com/crylabsorg/prostack-go/ps_nats"
)

type publisherMessage struct{}

func (publisherMessage) ID() string                      { return "event-id" }
func (publisherMessage) Subject() string                 { return "user.created" }
func (publisherMessage) Payload() []byte                 { return []byte(`{"id":"user-id"}`) }
func (publisherMessage) TraceContext() map[string]string { return nil }

type publisherJetStream struct {
	jetstream.JetStream

	publish func(context.Context, *nats.Msg) (*jetstream.PubAck, error)
}

func (js publisherJetStream) PublishMsg(
	ctx context.Context,
	msg *nats.Msg,
	_ ...jetstream.PublishOpt,
) (*jetstream.PubAck, error) {
	return js.publish(ctx, msg)
}

//nolint:paralleltest // The test temporarily replaces OpenTelemetry globals.
func TestPublisherPropagatesContextAndErrors(t *testing.T) {
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
	traceState, err := trace.ParseTraceState("vendor=value")
	require.NoError(t, err)
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{2},
		TraceFlags: trace.FlagsSampled,
		TraceState: traceState,
	})
	publishCtx := trace.ContextWithSpanContext(context.Background(), parent)
	publishBaggage, err := baggage.Parse("customer=acme")
	require.NoError(t, err)
	publishCtx = baggage.ContextWithBaggage(publishCtx, publishBaggage)

	for _, test := range []struct {
		name       string
		publishErr error
	}{
		{name: "success"},
		{name: "broker failure", publishErr: errors.New("nats rejected event")},
	} {
		t.Run(test.name, func(t *testing.T) {
			var injected trace.SpanContext
			publisher := ps_nats.NewPublisher(
				publisherJetStream{publish: func(ctx context.Context, message *nats.Msg) (*jetstream.PubAck, error) {
					require.Equal(t, "event-id", message.Header.Get(nats.MsgIdHdr))
					require.Equal(t, "user.created", message.Subject)
					require.JSONEq(t, `{"id":"user-id"}`, string(message.Data))
					for _, key := range []string{"traceparent", "tracestate", "baggage"} {
						require.Contains(t, message.Header, key)
					}
					for _, key := range []string{"Traceparent", "Tracestate", "Baggage"} {
						require.NotContains(t, message.Header, key)
					}
					extracted := otel.GetTextMapPropagator().Extract(context.Background(), propagation.MapCarrier{
						"traceparent": message.Header.Get("traceparent"),
						"tracestate":  message.Header.Get("tracestate"),
						"baggage":     message.Header.Get("baggage"),
					})
					injected = trace.SpanContextFromContext(extracted)
					require.True(t, injected.IsValid())
					require.Equal(t, trace.SpanContextFromContext(ctx).SpanID(), injected.SpanID())
					require.Equal(t, "vendor=value", injected.TraceState().String())
					require.Equal(t, "acme", baggage.FromContext(extracted).Member("customer").Value())
					require.True(t, trace.SpanFromContext(ctx).IsRecording(), "publish span must cover the broker call")
					return &jetstream.PubAck{}, test.publishErr
				}},
			)
			endedBefore := len(recorder.Ended())
			publishErr := publisher.Publish(publishCtx, publisherMessage{})
			if test.publishErr != nil {
				require.ErrorIs(t, publishErr, test.publishErr)
			} else {
				require.NoError(t, publishErr)
			}
			require.Len(t, recorder.Ended(), endedBefore+1)
			span := recorder.Ended()[endedBefore]
			require.Equal(t, parent, span.Parent())
			require.Equal(t, injected.SpanID(), span.SpanContext().SpanID())
			require.Equal(t, "publish user.created", span.Name())
			require.Equal(t, trace.SpanKindProducer, span.SpanKind())
			attributes := attribute.NewSet(span.Attributes()...)
			for key, value := range map[attribute.Key]string{
				"messaging.system":           "nats",
				"messaging.destination.name": "user.created",
				"messaging.message.id":       "event-id",
				"messaging.operation.name":   "publish",
				"messaging.operation.type":   "send",
			} {
				actual, ok := attributes.Value(key)
				require.True(t, ok, "missing attribute %s", key)
				require.Equal(t, value, actual.AsString())
			}
			if test.publishErr != nil {
				require.Equal(t, codes.Error, span.Status().Code)
				require.Equal(t, test.publishErr.Error(), span.Status().Description)
				errorType, ok := attributes.Value("error.type")
				require.True(t, ok)
				require.Equal(t, fmt.Sprintf("%T", test.publishErr), errorType.AsString())
				require.Len(t, span.Events(), 1)
				require.Equal(t, "exception", span.Events()[0].Name)
				eventAttributes := attribute.NewSet(span.Events()[0].Attributes...)
				errorMessage, ok := eventAttributes.Value("exception.message")
				require.True(t, ok)
				require.Equal(t, test.publishErr.Error(), errorMessage.AsString())
			} else {
				require.Equal(t, codes.Unset, span.Status().Code)
				require.Empty(t, span.Events())
				require.False(t, attributes.HasValue("error.type"))
			}
		})
	}
}
