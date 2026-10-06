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

func Consume(ctx context.Context, drainingCtx context.Context, consumer jetstream.Consumer, parallelism int, handle func(context.Context, jetstream.Msg) error) error {
	if parallelism <= 0 {
		return fmt.Errorf("parallelism must be greater than zero")
	}
	slots := make(chan struct{}, parallelism)
	for range parallelism {
		slots <- struct{}{}
	}
	var workers sync.WaitGroup
	defer workers.Wait()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-drainingCtx.Done():
			return nil
		default:
		}
		if len(slots) == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-drainingCtx.Done():
				return nil
			case <-slots:
				slots <- struct{}{}
			}
		}
		const maxWait = 5 * time.Second
		batch, err := consumer.Fetch(len(slots), jetstream.FetchMaxWait(maxWait))
		if err != nil {
			return err
		}
	receive:
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-drainingCtx.Done():
				return nil
			default:
			}

			select {
			case <-ctx.Done():
				return nil
			case <-drainingCtx.Done():
				return nil
			case msg, ok := <-batch.Messages():
				if !ok {
					break receive
				}
				<-slots
				workers.Go(func() {
					defer func() { slots <- struct{}{} }()
					msgCtx, span := traceContext(ctx, msg)
					defer span.End()
					if err1 := handle(msgCtx, msg); err1 != nil {
						span.RecordError(err1)
						logger.GetLogger(msgCtx).Error("failed to handle msg", zap.Error(err1))
					}
				})
			}
		}
		if err := batch.Error(); err != nil {
			return err
		}
	}
}
