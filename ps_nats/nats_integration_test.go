package ps_nats_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/crylabsorg/prostack-go/ps_nats"
	"github.com/crylabsorg/prostack-go/ps_test"
)

type observedIterator struct {
	jetstream.MessagesContext

	reading chan struct{}
	stopped chan struct{}
}

func (i observedIterator) Next(opts ...jetstream.NextOpt) (jetstream.Msg, error) {
	i.reading <- struct{}{}
	return i.MessagesContext.Next(opts...)
}

func (i observedIterator) Stop() {
	i.MessagesContext.Stop()
	close(i.stopped)
}

//nolint:gocognit,paralleltest,tparallel // Keep lifecycle assertions together; subtests share a stream and publish sequentially.
func TestRunConsumerCancelPendingRead(t *testing.T) {
	t.Parallel()
	connection, err := ps_nats.Connect(ps_test.NewNATSContainer(t))
	require.NoError(t, err)
	t.Cleanup(connection.Close)
	require.Equal(t, -1, connection.Opts.MaxReconnect)
	js, err := jetstream.New(connection)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	require.NoError(t, ps_nats.EnsureStream(ctx, js, "cancellation", "test"))
	for _, drain := range []bool{false, true} {
		for _, activeWorker := range []bool{false, true} {
			t.Run(fmt.Sprintf("drain=%t/active_worker=%t", drain, activeWorker), func(t *testing.T) {
				jsConsumer, createErr := js.CreateConsumer(ctx, "cancellation", jetstream.ConsumerConfig{
					AckPolicy:     jetstream.AckExplicitPolicy,
					DeliverPolicy: jetstream.DeliverNewPolicy,
				})
				require.NoError(t, createErr)
				if activeWorker {
					_, publishErr := js.Publish(ctx, "test", []byte("message"))
					require.NoError(t, publishErr)
				}
				workCtx, stopWork := context.WithCancel(ctx)
				defer stopWork()
				drainingCtx, stopDrain := context.WithCancel(ctx)
				defer stopDrain()
				reading := make(chan struct{}, 2)
				stopped := make(chan struct{})
				consumer := consumeTestConsumer{
					messages: func(opts ...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
						iterator, messagesErr := jsConsumer.Messages(opts...)
						return observedIterator{
							MessagesContext: iterator,
							reading:         reading,
							stopped:         stopped,
						}, messagesErr
					},
				}
				release := make(chan struct{})
				defer close(release)
				handlerContexts := make(chan context.Context, 1)
				result := make(chan error, 1)
				go func() {
					result <- ps_nats.RunConsumer(workCtx, drainingCtx, consumer, 2,
						func(msgCtx context.Context, _ jetstream.Msg) error {
							handlerContexts <- msgCtx
							<-release
							return nil
						})
				}()
				reads := 1
				if activeWorker {
					reads++
				}
				for range reads {
					select {
					case <-reading:
					case <-ctx.Done():
						t.Fatal("consumer did not start reading")
					}
				}
				if drain {
					stopDrain()
				} else {
					stopWork()
				}
				select {
				case <-stopped:
				case <-ctx.Done():
					t.Fatal("cancellation did not interrupt Next")
				}
				if activeWorker {
					select {
					case handlerCtx := <-handlerContexts:
						if drain {
							require.NoError(t, handlerCtx.Err())
						} else {
							require.ErrorIs(t, handlerCtx.Err(), context.Canceled)
						}
					case <-ctx.Done():
						t.Fatal("handler did not start")
					}
					require.Empty(t, result, "must wait for the active worker")
					release <- struct{}{}
				}
				select {
				case runErr := <-result:
					require.NoError(t, runErr)
				case <-ctx.Done():
					t.Fatal("consumer did not stop")
				}
			})
		}
	}
}
