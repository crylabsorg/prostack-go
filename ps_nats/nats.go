package ps_nats

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/crylabsorg/prostack-go/logger"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
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
	spanName := fmt.Sprintf("publish %s", subject)
	ctx, span := otel.Tracer("ps_nats").Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindProducer))
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

type Outbox interface {
	Next(context.Context) (OutboxMessage, error)
}

type OutboxMessage interface {
	Subject() string
	ID() string
	Payload() []byte
	Complete(context.Context) error
	Rollback(context.Context) error
}

func RunOutbox(ctx context.Context, js jetstream.JetStream, outbox Outbox) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := PublishOutboxOnce(ctx, js, outbox); err != nil && !errors.Is(err, ErrOutboxEmpty) {
			logger.GetLogger(ctx).Error("publish outbox", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

var ErrOutboxEmpty = errors.New("outbox is empty")

func PublishOutboxOnce(ctx context.Context, js jetstream.JetStream, outbox Outbox) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	message, err := outbox.Next(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if rollbackErr := message.Rollback(ctx); rollbackErr != nil {
			err = errors.Join(err, rollbackErr)
		}
	}()
	if err := Publish(ctx, js, message.Subject(), message.ID(), message.Payload()); err != nil {
		return fmt.Errorf("publish %s: %w", message.ID(), err)
	}
	return message.Complete(ctx)
}

func TraceContext(ctx context.Context, msg jetstream.Msg) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(msg.Headers()))
	return otel.Tracer("ps_nats").Start(ctx, "process "+msg.Subject(), trace.WithSpanKind(trace.SpanKindConsumer))
}

func Consume(ctx context.Context, consumer jetstream.Consumer, parallelism int, handle func(context.Context, jetstream.Msg)) error {
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
				handle(ctx, msg)
			})
		}
		if err := batch.Error(); err != nil {
			return err
		}
	}
	return nil
}
