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

// Outbox returns up to limit messages reserved for publishing.
// CompleteBatch may receive only the messages acknowledged by JetStream;
// the remaining messages stay in the outbox when the transaction commits.
type Outbox interface {
	NextBatch(context.Context) ([]OutboxMessage, error)
	Complete(context.Context, []OutboxMessage, []OutboxMessage, []error) error
}

type OutboxMessage interface {
	Subject() string
	ID() string
	Payload() []byte
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
			logger.GetLogger(ctx).Error("publish outbox", zap.Error(err))
		}
	}
}

// publishBatch publishes and completes one reserved batch.
func (op *outboxPublisher) publishBatch(ctx context.Context) error {
	messages, err := op.outbox.NextBatch(ctx)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	succeeded := make([]OutboxMessage, 0, len(messages))
	var failed []OutboxMessage
	var failedErrors []error
	for i, publishErr := range op.publishMessages(ctx, messages) {
		if publishErr == nil {
			succeeded = append(succeeded, messages[i])
		} else {
			failed = append(failed, messages[i])
			failedErrors = append(failedErrors, publishErr)
		}
	}
	return op.outbox.Complete(ctx, succeeded, failed, failedErrors)
}

// publishMessages returns one result per message, in input order. A nil result
// means JetStream acknowledged that message; the batch is not atomic.
func (op *outboxPublisher) publishMessages(ctx context.Context, messages []OutboxMessage) []error {
	if len(messages) == 0 {
		return nil
	}
	ctx, batchSpan := otel.Tracer("ps_nats").Start(ctx, "publish batch", trace.WithSpanKind(trace.SpanKindProducer))
	defer batchSpan.End()
	const maxConcurrentPublishes = 100
	results := make([]error, len(messages))
	semaphore := make(chan struct{}, maxConcurrentPublishes)
	workers := sync.WaitGroup{}
	for i, message := range messages {
		select {
		case <-ctx.Done():
			results[i] = fmt.Errorf("publish %s: %w", message.ID(), ctx.Err())
			continue
		default:
		}
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			results[i] = fmt.Errorf("publish %s: %w", message.ID(), ctx.Err())
			continue
		}
		workers.Go(func() {
			defer func() {
				<-semaphore
			}()
			spanCtx, span := otel.Tracer("ps_nats").Start(ctx, fmt.Sprintf("publish %s", message.Subject()), trace.WithSpanKind(trace.SpanKindProducer))
			defer span.End()
			msg := nats.NewMsg(message.Subject())
			msg.Data = message.Payload()
			msg.Header.Set(nats.MsgIdHdr, message.ID())
			otel.GetTextMapPropagator().Inject(spanCtx, propagation.HeaderCarrier(msg.Header))
			_, err := op.js.PublishMsg(spanCtx, msg)
			if err != nil {
				span.RecordError(err)
				results[i] = fmt.Errorf("publish %s: %w", message.ID(), err)
			}
		})
	}
	workers.Wait()
	return results
}
