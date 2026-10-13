package ps_kafka //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"fmt"
	"strconv"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/crylabsorg/prostack-go/ps_event"
)

type Publisher struct{ client *kgo.Client }

var _ ps_event.Publisher = (*Publisher)(nil)

func NewPublisher(brokers ...string) (*Publisher, error) {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.AllowAutoTopicCreation())
	if err != nil {
		return nil, err
	}
	return &Publisher{client: client}, nil
}

func (p *Publisher) Close() { p.client.Close() }

func (p *Publisher) Ping(ctx context.Context) error { return p.client.Ping(ctx) }

func (p *Publisher) Publish(ctx context.Context, message ps_event.OutboxMessage) error {
	ctx, span := otel.Tracer("ps_kafka").Start(
		ctx,
		"publish "+message.Subject(),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", message.Subject()),
			attribute.String("messaging.message.id", message.ID()),
		),
	)
	defer span.End()
	record := &kgo.Record{Topic: message.Subject(), Key: []byte(message.ID()), Value: message.Payload()}
	otel.GetTextMapPropagator().Inject(ctx, recordCarrier{record})
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("kafka publish %s: %w", message.ID(), err)
	}
	return nil
}

// RunConsumer processes one record at a time and commits only after success.
// On failure it closes the client without committing, so the next run retries
// the record. Draining stops polling and lets the current handler finish.
func RunConsumer(
	ctx, drainingCtx context.Context,
	brokers []string,
	topic, group string,
	handle func(context.Context, *kgo.Record) error,
) error {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.ConsumeTopics(topic),
		kgo.ConsumerGroup(group), kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll(),
		kgo.AllowAutoTopicCreation())
	if err != nil {
		return err
	}
	defer client.CloseAllowingRebalance()
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopDrain := context.AfterFunc(drainingCtx, cancel)
	defer stopDrain()
	for readCtx.Err() == nil {
		fetches := client.PollRecords(readCtx, 1)
		if readCtx.Err() != nil {
			return nil
		}
		if fetchErr := fetches.Err(); fetchErr != nil {
			return fmt.Errorf("kafka fetch %s: %w", topic, fetchErr)
		}
		for _, record := range fetches.Records() {
			if processErr := processRecord(ctx, record, handle, func(ctx context.Context, record *kgo.Record) error {
				return client.CommitRecords(ctx, record)
			}); processErr != nil {
				return processErr
			}
		}
		client.AllowRebalance()
	}
	return nil
}

func processRecord(
	ctx context.Context,
	record *kgo.Record,
	handle, commit func(context.Context, *kgo.Record) error,
) error {
	ctx = otel.GetTextMapPropagator().Extract(ctx, recordCarrier{record})
	publisherContext := trace.SpanContextFromContext(ctx)
	options := []trace.SpanStartOption{
		trace.WithNewRoot(),
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", record.Topic),
			attribute.String("messaging.message.id", string(record.Key)),
			attribute.String("messaging.destination.partition.id", strconv.Itoa(int(record.Partition))),
			attribute.Int64("messaging.kafka.offset", record.Offset),
		),
	}
	if publisherContext.IsValid() {
		options = append(options, trace.WithLinks(trace.Link{SpanContext: publisherContext}))
	}
	ctx, span := otel.Tracer("ps_kafka").Start(ctx, "process "+record.Topic, options...)
	defer span.End()
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

type recordCarrier struct{ record *kgo.Record }

func (c recordCarrier) Get(key string) string {
	for _, header := range c.record.Headers {
		if header.Key == key {
			return string(header.Value)
		}
	}
	return ""
}

func (c recordCarrier) Set(key, value string) {
	c.record.Headers = append(c.record.Headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
}

func (c recordCarrier) Keys() []string {
	keys := make([]string, len(c.record.Headers))
	for i, header := range c.record.Headers {
		keys[i] = header.Key
	}
	return keys
}
