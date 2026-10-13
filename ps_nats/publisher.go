package ps_nats

import (
	"context"
	"fmt"

	"github.com/crylabsorg/prostack-go/ps_event"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Publisher struct{ js jetstream.JetStream }

var _ ps_event.Publisher = (*Publisher)(nil)

func NewPublisher(js jetstream.JetStream) *Publisher { return &Publisher{js: js} }

func (p *Publisher) Publish(ctx context.Context, message ps_event.OutboxMessage) error {
	ctx, span := otel.Tracer("ps_nats").Start(
		ctx,
		"publish "+message.Subject(),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", message.Subject()),
			attribute.String("messaging.message.id", message.ID()),
		),
	)
	defer span.End()
	msg := nats.NewMsg(message.Subject())
	msg.Data = message.Payload()
	msg.Header.Set(nats.MsgIdHdr, message.ID())
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(msg.Header))
	if _, err := p.js.PublishMsg(ctx, msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("nats publish %s: %w", message.ID(), err)
	}
	return nil
}
