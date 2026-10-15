package ps_nats //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/crylabsorg/prostack-go/ps_event"
)

type Publisher struct{ js jetstream.JetStream }

var _ ps_event.Publisher = (*Publisher)(nil)

func NewPublisher(js jetstream.JetStream) *Publisher { return &Publisher{js: js} }

func (p *Publisher) Publish(ctx context.Context, message ps_event.OutboxMessage) error {
	ctx, span := otel.Tracer("ps_nats").Start(
		ctx,
		fmt.Sprintf("publish %s", message.Subject()),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", message.Subject()),
			attribute.String("messaging.message.id", message.ID()),
			attribute.String("messaging.operation.name", "publish"),
			attribute.String("messaging.operation.type", "send"),
		),
	)
	defer span.End()
	msg := nats.NewMsg(message.Subject())
	msg.Data = message.Payload()
	msg.Header.Set(nats.MsgIdHdr, message.ID())
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(msg.Header))
	if _, err := p.js.PublishMsg(ctx, msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err)))
		return fmt.Errorf("nats publish %s: %w", message.ID(), err)
	}
	return nil
}
