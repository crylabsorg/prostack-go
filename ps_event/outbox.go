package ps_event //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
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
// OutboxMessage must be non-nil. A nil Err means the publisher acknowledged the message,
// independently of whether the outbox record was successfully marked as processed.
type OutboxMessagePublishResult struct {
	OutboxMessage

	Err error
}

type outboxPublisher struct {
	publisher Publisher
	outbox    Outbox
	ticker    *time.Ticker
}

func RunOutboxPublisher(
	ctx context.Context,
	drainingCtx context.Context,
	publisher Publisher,
	outbox Outbox,
) <-chan struct{} {
	op := outboxPublisher{publisher: publisher, outbox: outbox, ticker: time.NewTicker(time.Second)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		op.run(ctx, drainingCtx)
	}()
	return done
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
	const batchTimeout = 30 * time.Second
	ctx, cancel := context.WithTimeout(ctx, batchTimeout)
	defer cancel()
	return op.outbox.ProcessOutboxBatch(ctx, func(messages []OutboxMessage) []OutboxMessagePublishResult {
		results := op.publishMessages(ctx, messages)
		for _, result := range results {
			if result.Err != nil {
				logger.GetLogger(ctx).
					Error("publish outbox message", zap.String("id", result.ID()), zap.String("subject", result.Subject()), zap.Error(result.Err))
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
	ctx, batchSpan := otel.Tracer("ps_event").Start(ctx, "publish batch", trace.WithSpanKind(trace.SpanKindProducer))
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
			sourceCtx := propagation.TraceContext{}.Extract(
				context.Background(),
				propagation.MapCarrier(message.TraceContext()),
			)
			sourceSpanContext := trace.SpanContextFromContext(sourceCtx)
			options := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindProducer)}
			if sourceSpanContext.IsValid() {
				options = append(options, trace.WithLinks(trace.Link{SpanContext: sourceSpanContext}))
			}
			spanCtx, span := otel.Tracer("ps_event").
				Start(ctx, fmt.Sprintf("publish %s", message.Subject()), options...)
			defer span.End()
			err := op.publisher.Publish(spanCtx, message)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				results[i].Err = fmt.Errorf("publish %s: %w", message.ID(), err)
			}
		})
	}
	workers.Wait()
	return results
}
