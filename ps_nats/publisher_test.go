package ps_nats

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
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

func (js publisherJetStream) PublishMsg(ctx context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	return js.publish(ctx, msg)
}

func TestPublisherPropagatesContextAndErrors(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	publishErr := errors.New("nats rejected event")
	publisher := NewPublisher(publisherJetStream{publish: func(ctx context.Context, message *nats.Msg) (*jetstream.PubAck, error) {
		require.Equal(t, "event-id", message.Header.Get(nats.MsgIdHdr))
		require.Equal(t, "user.created", message.Subject)
		require.JSONEq(t, `{"id":"user-id"}`, string(message.Data))
		extracted := propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(message.Header))
		require.True(t, trace.SpanContextFromContext(extracted).IsValid())
		require.Equal(t, trace.SpanContextFromContext(ctx).SpanID(), trace.SpanContextFromContext(extracted).SpanID())
		return nil, publishErr
	}})
	require.ErrorIs(t, publisher.Publish(context.Background(), publisherMessage{}), publishErr)
}
