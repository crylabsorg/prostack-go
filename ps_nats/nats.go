package ps_nats

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	const duplicateWindow = 15 * time.Minute
	jsConfig := jetstream.StreamConfig{
		Name:       name,
		Subjects:   subjects,
		Storage:    jetstream.FileStorage,
		Duplicates: duplicateWindow,
	}
	_, err := js.CreateStream(ctx, jsConfig)
	if !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		return err
	}
	return nil
}

func traceContext(ctx context.Context, msg jetstream.Msg) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(msg.Headers()))
	spanName := fmt.Sprintf("process %s", msg.Subject())
	return otel.Tracer("ps_nats").Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindConsumer))
}

func Consume(ctx context.Context, consumer jetstream.Consumer, parallelism int, handle func(context.Context, jetstream.Msg) error) error {
	slots := make(chan struct{}, parallelism)
	for range parallelism {
		slots <- struct{}{}
	}
	var workers sync.WaitGroup
	defer workers.Wait()
	for ctx.Err() == nil {
		if len(slots) == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-slots:
				slots <- struct{}{}
			}
		}
		batch, err := consumer.Fetch(len(slots), jetstream.FetchMaxWait(time.Second))
		if err != nil {
			return err
		}
		for msg := range batch.Messages() {
			<-slots
			workers.Go(func() {
				defer func() { slots <- struct{}{} }()
				msgCtx, span := traceContext(ctx, msg)
				defer span.End()
				if err := handle(msgCtx, msg); err != nil {
					span.RecordError(err)
				}
			})
		}
		if err := batch.Error(); err != nil {
			return err
		}
	}
	return nil
}
