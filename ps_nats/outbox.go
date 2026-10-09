package ps_nats

import (
	"context"
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

// Outbox reserves messages while publish runs and marks only acknowledged messages as processed.
// ProcessOutboxBatch reports storage failures.
// publish must return one result per input message, containing that original message.
type Outbox interface {
	ProcessOutboxBatch(ctx context.Context, publish func([]OutboxMessage) []OutboxMessagePublishResult) error
}

type OutboxMessage interface {
	Subject() string
	ID() string
	Payload() []byte
	TraceContext() map[string]string
}

// OutboxMessagePublishResult contains the original message and its publication error.
// OutboxMessage must be non-nil. A nil Err means JetStream acknowledged the message,
// independently of whether the outbox record was successfully marked as processed.
type OutboxMessagePublishResult struct {
	OutboxMessage
	Err error
}

type outboxPublisher struct {
	js     jetstream.JetStream
	outbox Outbox
	ticker *time.Ticker
}

func RunOutboxPublisher(ctx context.Context, drainingCtx context.Context, js jetstream.JetStream, outbox Outbox) {
	op := outboxPublisher{js: js, outbox: outbox, ticker: time.NewTicker(time.Second)}
	go op.run(ctx, drainingCtx)
}

func (op *outboxPublisher) run(ctx context.Context, drainingCtx context.Context) {
	defer op.ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-drainingCtx.Done():
			return
		default:
		}

		select {
		case <-ctx.Done():
			return
		case <-drainingCtx.Done():
			return
		case <-op.ticker.C:
		}

		if err := op.publishBatch(ctx); err != nil {
			logger.GetLogger(ctx).Error("process outbox batch", zap.Error(err))
		}
	}
}

// publishBatch publishes and completes one reserved batch.
func (op *outboxPublisher) publishBatch(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return op.outbox.ProcessOutboxBatch(ctx, func(messages []OutboxMessage) []OutboxMessagePublishResult {
		results := op.publishMessages(ctx, messages)
		for _, result := range results {
			if result.Err != nil {
				logger.GetLogger(ctx).Error("publish outbox message", zap.String("id", result.ID()), zap.String("subject", result.Subject()), zap.Error(result.Err))
			}
		}
		return results
	})
}

// publishMessages returns one result per message, in input order.
// The batch is not atomic: each message is acknowledged independently.
func (op *outboxPublisher) publishMessages(ctx context.Context, messages []OutboxMessage) []OutboxMessagePublishResult {
	if len(messages) == 0 {
		return nil
	}
	ctx, batchSpan := otel.Tracer("ps_nats").Start(ctx, "publish batch", trace.WithSpanKind(trace.SpanKindProducer))
	defer batchSpan.End()
	const maxConcurrentPublishes = 100
	results := make([]OutboxMessagePublishResult, len(messages))
	semaphore := make(chan struct{}, maxConcurrentPublishes)
	workers := sync.WaitGroup{}
	for i, message := range messages {
		results[i].OutboxMessage = message
		select {
		case <-ctx.Done():
			results[i].Err = fmt.Errorf("publish %s: %w", message.ID(), ctx.Err())
			continue
		default:
		}
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			results[i].Err = fmt.Errorf("publish %s: %w", message.ID(), ctx.Err())
			continue
		}
		workers.Go(func() {
			defer func() {
				<-semaphore
			}()
			sourceCtx := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier(message.TraceContext()))
			sourceSpanContext := trace.SpanContextFromContext(sourceCtx)
			options := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindProducer)}
			if sourceSpanContext.IsValid() {
				options = append(options, trace.WithLinks(trace.Link{SpanContext: sourceSpanContext}))
			}
			spanCtx, span := otel.Tracer("ps_nats").Start(ctx, fmt.Sprintf("publish %s", message.Subject()), options...)
			defer span.End()
			msg := nats.NewMsg(message.Subject())
			msg.Data = message.Payload()
			msg.Header.Set(nats.MsgIdHdr, message.ID())
			otel.GetTextMapPropagator().Inject(spanCtx, propagation.HeaderCarrier(msg.Header))
			_, err := op.js.PublishMsg(spanCtx, msg)
			if err != nil {
				span.RecordError(err)
				results[i].Err = fmt.Errorf("publish %s: %w", message.ID(), err)
			}
		})
	}
	workers.Wait()
	return results
}
