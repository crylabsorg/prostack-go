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

// publishMessages returns one result per message, in input order. A nil result
// means JetStream acknowledged that message; the batch is not atomic.
func (p *OutboxPublisher) publishMessages(ctx context.Context, messages []OutboxMessage) []error {
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
			_, err := p.JS.PublishMsg(spanCtx, msg)
			if err != nil {
				span.RecordError(err)
				results[i] = fmt.Errorf("publish %s: %w", message.ID(), err)
			}
		})
	}
	workers.Wait()
	var result error
	for _, publishErr := range results {
		result = errors.Join(result, publishErr)
	}
	if result != nil {
		batchSpan.RecordError(result)
	}
	return results
}

// Outbox returns up to limit messages reserved for publishing.
// CompleteBatch may receive only the messages acknowledged by JetStream;
// the remaining messages stay in the outbox when the transaction commits.
type Outbox interface {
	NextBatch(context.Context, int) ([]OutboxMessage, error)
	CompleteBatch(context.Context, []OutboxMessage) error
	RollbackBatch(context.Context, []OutboxMessage) error
}

type OutboxMessage interface {
	Subject() string
	ID() string
	Payload() []byte
}

type OutboxPublisher struct {
	JS     jetstream.JetStream
	Outbox Outbox
}

func (p *OutboxPublisher) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		for ctx.Err() == nil {
			completed, err := p.publishBatch(ctx, 100)
			if errors.Is(err, ErrOutboxEmpty) {
				break
			}
			if err != nil {
				logger.GetLogger(ctx).Error("publish outbox", zap.Error(err))
				if completed == 0 {
					break
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

var ErrOutboxEmpty = errors.New("outbox is empty")

func (p *OutboxPublisher) PublishOnce(ctx context.Context) error {
	_, err := p.publishBatch(ctx, 1)
	return err
}

// publishBatch publishes and completes one reserved batch.
func (p *OutboxPublisher) publishBatch(ctx context.Context, limit int) (completedCount int, err error) {
	if limit < 1 {
		return 0, fmt.Errorf("outbox batch size must be positive: %d", limit)
	}
	nextCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	messages, err := p.Outbox.NextBatch(nextCtx, limit)
	cancel()
	if err != nil {
		return 0, err
	}
	if len(messages) == 0 {
		return 0, ErrOutboxEmpty
	}
	completed := false
	defer func() {
		if !completed {
			rollbackCtx, rollbackCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer rollbackCancel()
			err = errors.Join(err, p.Outbox.RollbackBatch(rollbackCtx, messages))
		}
	}()
	succeeded := make([]OutboxMessage, 0, len(messages))
	for i, publishErr := range p.publishMessages(ctx, messages) {
		if publishErr == nil {
			succeeded = append(succeeded, messages[i])
		} else {
			err = errors.Join(err, publishErr)
		}
	}
	if len(succeeded) == 0 {
		return 0, err
	}
	if completeErr := p.Outbox.CompleteBatch(ctx, succeeded); completeErr != nil {
		return 0, errors.Join(err, fmt.Errorf("complete outbox batch: %w", completeErr))
	}
	completed = true
	return len(succeeded), err
}
