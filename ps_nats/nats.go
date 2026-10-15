package ps_nats //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
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

// NATS preserves header casing. Read legacy HTTP-style keys as well as the
// lowercase propagation keys emitted by OpenTelemetry.
type headerCarrier nats.Header

func (c headerCarrier) Get(key string) string {
	values := c.Values(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (c headerCarrier) Values(key string) []string {
	if values, ok := c[key]; ok {
		return values
	}
	for header, values := range c {
		if strings.EqualFold(header, key) {
			return values
		}
	}
	return nil
}

func (c headerCarrier) Set(key, value string) { c[key] = []string{value} }

func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}
	return keys
}

func newConsumer(
	jsConsumer jetstream.Consumer,
	parallelism int,
	handle func(context.Context, jetstream.Msg) error,
) (*consumer, error) {
	if parallelism <= 0 {
		return nil, fmt.Errorf("parallelism must be greater than zero")
	}
	slots := make(chan struct{}, parallelism)
	for range parallelism {
		slots <- struct{}{}
	}
	return &consumer{consumer: jsConsumer, handle: handle, slots: slots}, nil
}

//nolint:spancheck // The caller owns the returned span and ends it after processing the message.
func (c *consumer) traceContext(ctx context.Context, msg jetstream.Msg) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, headerCarrier(msg.Headers()))
	publisherContext := trace.SpanContextFromContext(ctx)
	options := []trace.SpanStartOption{
		trace.WithNewRoot(),
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", msg.Subject()),
			attribute.String("messaging.operation.name", "process"),
			attribute.String("messaging.operation.type", "process"),
		),
	}
	if messageID := msg.Headers().Get(nats.MsgIdHdr); messageID != "" {
		options = append(options, trace.WithAttributes(attribute.String("messaging.message.id", messageID)))
	}
	if publisherContext.IsValid() {
		options = append(options, trace.WithLinks(trace.Link{SpanContext: publisherContext}))
	}
	spanName := fmt.Sprintf("process %s", msg.Subject())
	ctx, span := otel.Tracer("ps_nats").Start(ctx, spanName, options...)
	return ctx, span
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
		msg, nextErr := msgs.Next(jetstream.NextContext(readCtx))
		if nextErr != nil {
			if errors.Is(nextErr, jetstream.ErrMsgIteratorClosed) || errors.Is(readCtx.Err(), context.Canceled) {
				return nil
			}
			return nextErr
		}
		c.workers.Go(func() {
			defer func() { c.slots <- struct{}{} }()
			msgCtx, span := c.traceContext(ctx, msg)
			defer span.End()
			if err1 := c.handle(msgCtx, msg); err1 != nil {
				span.RecordError(err1)
				span.SetStatus(codes.Error, err1.Error())
				span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err1)))
				logger.GetLogger(msgCtx).Error("failed to handle msg", zap.Error(err1))
			}
		})
	}
}
