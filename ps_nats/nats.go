package ps_nats

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func Connect(url string) (*nats.Conn, error) {
	return nats.Connect(url, nats.MaxReconnects(-1))
}

func EnsureStream(ctx context.Context, js jetstream.JetStream, name string, subjects ...string) error {
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: name, Subjects: subjects, Storage: jetstream.FileStorage, Duplicates: 24 * time.Hour})
	if errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		return nil
	}
	return err
}

func Publish(ctx context.Context, js jetstream.JetStream, subject, id string, payload []byte) error {
	ctx, span := otel.Tracer("ps_nats").Start(ctx, "publish "+subject, trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()
	msg := nats.NewMsg(subject)
	msg.Data = payload
	msg.Header.Set(nats.MsgIdHdr, id)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(msg.Header))
	_, err := js.PublishMsg(ctx, msg)
	if err != nil {
		span.RecordError(err)
	}
	return err
}

func TraceContext(ctx context.Context, msg jetstream.Msg) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(msg.Headers()))
	return otel.Tracer("ps_nats").Start(ctx, "process "+msg.Subject(), trace.WithSpanKind(trace.SpanKindConsumer))
}
