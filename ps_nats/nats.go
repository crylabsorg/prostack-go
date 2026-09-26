package ps_nats

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func Connect(url string) (*nats.Conn, error) {
	return nats.Connect(url, nats.MaxReconnects(-1))
}

func EnsureStream(js nats.JetStreamContext, name string, subjects ...string) error {
	_, err := js.AddStream(&nats.StreamConfig{Name: name, Subjects: subjects, Storage: nats.FileStorage, Duplicates: 24 * time.Hour})
	if err == nats.ErrStreamNameAlreadyInUse {
		return nil
	}
	return err
}

func Publish(ctx context.Context, js nats.JetStreamContext, subject, id string, payload []byte) error {
	ctx, span := otel.Tracer("ps_nats").Start(ctx, "publish "+subject, trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()
	msg := nats.NewMsg(subject)
	msg.Data = payload
	msg.Header.Set(nats.MsgIdHdr, id)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(msg.Header))
	_, err := js.PublishMsg(msg, nats.Context(ctx))
	if err != nil {
		span.RecordError(err)
	}
	return err
}

func TraceContext(ctx context.Context, msg *nats.Msg) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(msg.Header))
	return otel.Tracer("ps_nats").Start(ctx, "process "+msg.Subject, trace.WithSpanKind(trace.SpanKindConsumer))
}
