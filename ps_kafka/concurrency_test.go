package ps_kafka_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/crylabsorg/prostack-go/ps_kafka"
	"github.com/crylabsorg/prostack-go/ps_test"
)

func TestKafkaConcurrentDeliveryAndRetry(t *testing.T) {
	t.Parallel()
	brokers := ps_test.NewKafkaContainer(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	producer, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.AllowAutoTopicCreation(),
		kgo.RecordPartitioner(kgo.ManualPartitioner()))
	require.NoError(t, err)
	t.Cleanup(producer.Close)
	const topic = "concurrent-delivery"
	require.NoError(t, producer.ProduceSync(ctx,
		&kgo.Record{Topic: topic, Partition: 0, Value: []byte("first")},
		&kgo.Record{Topic: topic, Partition: 0, Value: []byte("failure")},
		&kgo.Record{Topic: topic, Partition: 0, Value: []byte("later")},
	).FirstErr())

	started := make(chan int64, 3)
	releaseFirst := make(chan struct{})
	finished := make(chan error, 1)
	done := make(chan struct{})
	handleErr := errors.New("temporary delivery failure")
	go func() {
		defer close(done)
		finished <- ps_kafka.RunConsumer(ctx, ctx, brokers, topic, "concurrent-test",
			func(ctx context.Context, record *kgo.Record) error {
				started <- record.Offset
				switch record.Offset {
				case 0:
					select {
					case <-releaseFirst:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				case 1:
					return handleErr
				default:
					return nil
				}
			})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	var offsets []int64
	for range 3 {
		select {
		case offset := <-started:
			offsets = append(offsets, offset)
		case <-ctx.Done():
			t.Fatal("records in a single partition were not handled concurrently")
		}
	}
	require.ElementsMatch(t, []int64{0, 1, 2}, offsets)
	close(releaseFirst)
	require.ErrorIs(t, <-finished, handleErr)

	drainingCtx, drain := context.WithCancel(ctx)
	defer drain()
	redelivered := make(chan int64, 3)
	var attempts atomic.Int32
	err = ps_kafka.RunConsumer(ctx, drainingCtx, brokers, topic, "concurrent-test",
		func(_ context.Context, record *kgo.Record) error {
			redelivered <- record.Offset
			if attempts.Add(1) == 2 {
				drain()
			}
			return nil
		})
	require.NoError(t, err)
	require.NoError(t, ctx.Err(), "retry and drain must finish before the timeout")
	require.Len(t, redelivered, 2)
	require.ElementsMatch(t, []int64{1, 2}, []int64{<-redelivered, <-redelivered},
		"offset 0 must be committed; the failure and every later offset must be retried")
}
