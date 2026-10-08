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

func RunConsumer(
	ctx context.Context,
	drainingCtx context.Context,
	jsConsumer jetstream.Consumer,
	parallelism int,
	handle func(context.Context, jetstream.Msg) error,
) error {
	consumer, err := newConsumer(jsConsumer, parallelism, handle)
	if err != nil {
		return err
	}
	return consumer.run(ctx, drainingCtx)
}

type consumer struct {
	consumer jetstream.Consumer
	handle   func(context.Context, jetstream.Msg) error
	slots    chan struct{}
	workers  sync.WaitGroup
}

func newConsumer(jsConsumer jetstream.Consumer, parallelism int, handle func(context.Context, jetstream.Msg) error) (*consumer, error) {
	if parallelism <= 0 {
		return nil, fmt.Errorf("parallelism must be greater than zero")
	}
	slots := make(chan struct{}, parallelism)
	for range parallelism {
		slots <- struct{}{}
	}
	return &consumer{consumer: jsConsumer, handle: handle, slots: slots}, nil
}

func (c *consumer) traceContext(ctx context.Context, msg jetstream.Msg) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(msg.Headers()))
	spanName := fmt.Sprintf("process %s", msg.Subject())
	return otel.Tracer("ps_nats").Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindConsumer))
}

func (c *consumer) run(ctx context.Context, drainingCtx context.Context) error {
	msgs, err := c.consumer.Messages(jetstream.PullMaxMessages(cap(c.slots)))
	if err != nil {
		return err
	}
	defer c.workers.Wait()
	defer msgs.Stop()

	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopDrain := context.AfterFunc(drainingCtx, cancel)
	defer stopDrain()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-drainingCtx.Done():
			return nil
		case <-c.slots:
		}
		msg, err := msgs.Next(jetstream.NextContext(readCtx))
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgIteratorClosed) || errors.Is(readCtx.Err(), context.Canceled) {
				return nil
			}
			return err
		}
		c.workers.Go(func() {
			defer func() { c.slots <- struct{}{} }()
			msgCtx, span := c.traceContext(ctx, msg)
			defer span.End()
			if err1 := c.handle(msgCtx, msg); err1 != nil {
				span.RecordError(err1)
				logger.GetLogger(msgCtx).Error("failed to handle msg", zap.Error(err1))
			}
		})
	}
}
