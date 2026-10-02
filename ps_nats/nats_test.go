package ps_nats

import (
	"context"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type consumeTestConsumer struct {
	jetstream.Consumer
	fetch func(int) (jetstream.MessageBatch, error)
}

func (c consumeTestConsumer) Fetch(size int, _ ...jetstream.FetchOpt) (jetstream.MessageBatch, error) {
	return c.fetch(size)
}

type consumeTestBatch struct {
	messages chan jetstream.Msg
	err      error
}

func (b consumeTestBatch) Messages() <-chan jetstream.Msg { return b.messages }
func (b consumeTestBatch) Error() error                   { return b.err }

type consumeTestMessage struct{ jetstream.Msg }

func (consumeTestMessage) Headers() nats.Header { return nil }
func (consumeTestMessage) Subject() string      { return "test" }

func TestConsume(t *testing.T) {
	t.Run("invalid parallelism", func(t *testing.T) {
		for _, size := range []int{0, -1} {
			require.Error(t, Consume(context.Background(), nil, size, nil))
		}
	})
	t.Run("closed batch and workers", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		batchErr := errors.New("batch failed")
		messages := make(chan jetstream.Msg, 2)
		messages <- consumeTestMessage{}
		messages <- consumeTestMessage{}
		close(messages)
		handled := make(chan struct{}, 2)
		consumer := consumeTestConsumer{fetch: func(size int) (jetstream.MessageBatch, error) {
			require.Equal(t, 2, size)
			return consumeTestBatch{messages: messages, err: batchErr}, nil
		}}
		err := Consume(ctx, consumer, 2, func(context.Context, jetstream.Msg) error { handled <- struct{}{}; return nil })
		require.ErrorIs(t, err, batchErr)
		require.Len(t, handled, 2)
	})
	t.Run("cancel pending batch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		consumer := consumeTestConsumer{fetch: func(int) (jetstream.MessageBatch, error) {
			cancel()
			return consumeTestBatch{messages: make(chan jetstream.Msg)}, nil
		}}
		require.NoError(t, Consume(ctx, consumer, 1, nil))
	})
}
