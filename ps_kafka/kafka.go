package ps_kafka //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/crylabsorg/prostack-go/ps_event"
)

type Publisher struct{ client *kgo.Client }

var _ ps_event.Publisher = (*Publisher)(nil)

func NewPublisher(brokers ...string) (*Publisher, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
		kgo.WithHooks(kotel.NewKotel(kotel.WithTracer(kotel.NewTracer())).Hooks()...),
	)
	if err != nil {
		return nil, err
	}
	return &Publisher{client: client}, nil
}

func (p *Publisher) Close() { p.client.Close() }

func (p *Publisher) Ping(ctx context.Context) error { return p.client.Ping(ctx) }

func (p *Publisher) Publish(ctx context.Context, message ps_event.OutboxMessage) error {
	record := &kgo.Record{Topic: message.Subject(), Key: []byte(message.ID()), Value: message.Payload()}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("kafka publish %s: %w", message.ID(), err)
	}
	return nil
}

// RunConsumer processes up to 50 records concurrently. Handlers may finish out
// of order, but commits stay ordered within each partition and stop at failures.
// On failure the client closes and uncommitted records are retried on the next run.
// Draining stops polling and lets the current batch finish before closing.
func RunConsumer(
	ctx, drainingCtx context.Context,
	brokers []string,
	topic, group string,
	handle func(context.Context, *kgo.Record) error,
) error {
	tracer := kotel.NewTracer(kotel.ConsumerGroup(group), kotel.LinkSpans())
	client, err := kgo.NewClient(
		kgo.WithHooks(kotel.NewKotel(kotel.WithTracer(tracer)).Hooks()...),
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumerGroup(group),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return err
	}
	defer client.CloseAllowingRebalance()
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopDrain := context.AfterFunc(drainingCtx, cancel)
	defer stopDrain()
	const maxConcurrentRecords = 50
	for readCtx.Err() == nil {
		fetches := client.PollRecords(readCtx, maxConcurrentRecords)
		if readCtx.Err() != nil {
			return nil
		}
		if fetchErr := fetches.Err(); fetchErr != nil {
			return fmt.Errorf("kafka fetch %s: %w", topic, fetchErr)
		}
		if processErr := processRecords(
			ctx,
			fetches.Records(),
			group,
			handle,
			func(ctx context.Context, record *kgo.Record) error {
				return client.CommitRecords(ctx, record)
			},
		); processErr != nil {
			return processErr
		}
		client.AllowRebalance()
	}
	return nil
}

// processRecords handles a polled batch from one topic. Each commit waits for
// the preceding record in its partition to finish, including its commit.
func processRecords(
	ctx context.Context,
	records []*kgo.Record,
	group string,
	handle, commit func(context.Context, *kgo.Record) error,
) error {
	type result struct {
		done chan struct{}
		err  error
	}
	results := make([]result, len(records))
	partitions := make(map[int32]*result)
	var workers sync.WaitGroup
	for i, record := range records {
		previous := partitions[record.Partition]
		current := &results[i]
		current.done = make(chan struct{})
		partitions[record.Partition] = current
		workers.Go(func() {
			defer close(current.done)
			current.err = processRecord(
				ctx,
				record,
				group,
				handle,
				func(commitCtx context.Context, record *kgo.Record) error {
					if previous != nil {
						select {
						case <-previous.done:
						case <-commitCtx.Done():
							return commitCtx.Err()
						}
						if previous.err != nil {
							return errors.New("offset commit blocked by an earlier failed record in this partition")
						}
					}
					return commit(commitCtx, record)
				},
			)
		})
	}
	workers.Wait()
	var batchErr error
	for _, result := range results {
		batchErr = errors.Join(batchErr, result.err)
	}
	return batchErr
}

func processRecord(
	ctx context.Context,
	record *kgo.Record,
	group string,
	handle, commit func(context.Context, *kgo.Record) error,
) error {
	processRecord := *record
	processRecord.Context = otel.GetTextMapPropagator().Extract(
		trace.ContextWithSpanContext(ctx, trace.SpanContext{}), kotel.NewRecordCarrier(record),
	)
	processCtx, span := kotel.NewTracer(kotel.ConsumerGroup(group), kotel.LinkSpans()).WithProcessSpan(&processRecord)
	defer span.End()
	ctx = trace.ContextWithSpan(ctx, span)
	ctx = baggage.ContextWithBaggage(ctx, baggage.FromContext(processCtx))
	err := handle(ctx, record)
	if err == nil {
		err = commit(ctx, record)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf(
			"kafka process %s partition %d offset %d: %w",
			record.Topic,
			record.Partition,
			record.Offset,
			err,
		)
	}
	return nil
}
