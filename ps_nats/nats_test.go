package ps_nats_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/crylabsorg/prostack-go/ps_nats"
)

type consumeTestConsumer struct {
	jetstream.Consumer

	messages func(...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error)
}

func (c consumeTestConsumer) Messages(opts ...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
	return c.messages(opts...)
}

type consumeTestIterator struct {
	jetstream.MessagesContext

	next    func() (jetstream.Msg, error)
	stopped atomic.Bool
}

func (i *consumeTestIterator) Next(...jetstream.NextOpt) (jetstream.Msg, error) { return i.next() }
func (i *consumeTestIterator) Stop()                                            { i.stopped.Store(true) }

type consumeTestMessage struct{ jetstream.Msg }

func (consumeTestMessage) Headers() nats.Header { return nil }
func (consumeTestMessage) Subject() string      { return "test" }

func TestRunConsumerInvalidParallelism(t *testing.T) {
	t.Parallel()
	for _, parallelism := range []int{0, -1} {
		t.Run(strconv.Itoa(parallelism), func(t *testing.T) {
			t.Parallel()
			err := ps_nats.RunConsumer(t.Context(), t.Context(), nil, parallelism, nil)
			require.EqualError(t, err, "parallelism must be greater than zero")
		})
	}
}

func TestRunConsumerMessagesError(t *testing.T) {
	t.Parallel()
	messagesErr := errors.New("messages failed")
	consumer := consumeTestConsumer{
		messages: func(opts ...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
			require.Equal(t, []jetstream.PullMessagesOpt{jetstream.PullMaxMessages(2)}, opts)
			return nil, messagesErr
		},
	}
	require.ErrorIs(t, ps_nats.RunConsumer(t.Context(), t.Context(), consumer, 2, nil), messagesErr)
}

func TestRunConsumerIteratorErrors(t *testing.T) {
	t.Parallel()
	nextErr := errors.New("next failed")
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "closed", err: jetstream.ErrMsgIteratorClosed},
		{name: "wrapped closed", err: fmt.Errorf("next: %w", jetstream.ErrMsgIteratorClosed)},
		{name: "failure", err: nextErr, want: nextErr},
		{name: "deadline", err: context.DeadlineExceeded, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			iterator := &consumeTestIterator{next: func() (jetstream.Msg, error) { return nil, test.err }}
			consumer := consumeTestConsumer{
				messages: func(...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
					return iterator, nil
				},
			}
			err := ps_nats.RunConsumer(t.Context(), t.Context(), consumer, 1, nil)
			if test.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.want)
			}
			require.True(t, iterator.stopped.Load())
		})
	}
}

func TestRunConsumerParallelism(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		const parallelism = 2
		nextErr := errors.New("iterator failed")
		nextCalls := 0
		iterator := &consumeTestIterator{next: func() (jetstream.Msg, error) {
			nextCalls++
			if nextCalls > parallelism+1 {
				return nil, nextErr
			}
			return consumeTestMessage{}, nil
		}}
		consumer := consumeTestConsumer{
			messages: func(opts ...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
				require.Equal(t, []jetstream.PullMessagesOpt{jetstream.PullMaxMessages(parallelism)}, opts)
				return iterator, nil
			},
		}
		release := make(chan struct{})
		defer close(release)
		started := make(chan jetstream.Msg, parallelism+1)
		finished := make(chan struct{}, parallelism+1)
		result := make(chan error, 1)
		go func() {
			result <- ps_nats.RunConsumer(context.Background(), context.Background(), consumer, parallelism,
				func(_ context.Context, msg jetstream.Msg) error {
					started <- msg
					<-release
					finished <- struct{}{}
					return errors.New("handler failed")
				})
		}()
		synctest.Wait()
		require.Len(t, started, parallelism)
		require.Equal(t, parallelism, nextCalls, "must not read ahead while all workers are busy")
		release <- struct{}{}
		synctest.Wait()
		require.Len(t, started, parallelism+1, "handler errors must not stop consumption")
		require.Len(t, finished, 1)
		require.Empty(t, result)
		release <- struct{}{}
		synctest.Wait()
		require.True(t, iterator.stopped.Load())
		require.Empty(t, result, "iterator failure must still wait for active workers")
		release <- struct{}{}
		synctest.Wait()
		require.Len(t, result, 1)
		require.ErrorIs(t, <-result, nextErr)
		require.Len(t, finished, parallelism+1)
	})
}

func TestRunConsumerCancellation(t *testing.T) {
	t.Parallel()
	for _, drain := range []bool{false, true} {
		name := "context"
		if drain {
			name = "drain"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Run("before next", func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				workCtx, drainingCtx := ctx, t.Context()
				if drain {
					workCtx, drainingCtx = drainingCtx, workCtx
				}
				messagesCalls := 0
				nextCalls := 0
				iterator := &consumeTestIterator{next: func() (jetstream.Msg, error) {
					nextCalls++
					return nil, jetstream.ErrMsgIteratorClosed
				}}
				consumer := consumeTestConsumer{
					messages: func(...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
						messagesCalls++
						return iterator, nil
					},
				}
				require.NoError(t, ps_nats.RunConsumer(workCtx, drainingCtx, consumer, 1, nil))
				require.Zero(t, messagesCalls)
				require.Zero(t, nextCalls)
				require.False(t, iterator.stopped.Load())
			})
			t.Run("active worker", func(t *testing.T) {
				t.Parallel()
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					workCtx, drainingCtx := ctx, context.Background()
					if drain {
						workCtx, drainingCtx = drainingCtx, workCtx
					}
					nextCalls := 0
					iterator := &consumeTestIterator{next: func() (jetstream.Msg, error) {
						nextCalls++
						return consumeTestMessage{}, nil
					}}
					consumer := consumeTestConsumer{
						messages: func(...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
							return iterator, nil
						},
					}
					release := make(chan struct{})
					defer close(release)
					var handlerCtx context.Context
					finished := false
					result := make(chan error, 1)
					go func() {
						result <- ps_nats.RunConsumer(workCtx, drainingCtx, consumer, 1,
							func(ctx context.Context, _ jetstream.Msg) error {
								handlerCtx = ctx //nolint:fatcontext // Capture the handler context to verify cancellation semantics.
								<-release
								finished = true
								return nil
							})
					}()
					synctest.Wait()
					require.NotNil(t, handlerCtx)
					cancel()
					synctest.Wait()
					require.True(t, iterator.stopped.Load())
					require.Empty(t, result, "must wait for the active worker")
					require.False(t, finished)
					if drain {
						require.NoError(t, handlerCtx.Err(), "draining must not cancel the worker context")
					} else {
						require.ErrorIs(t, handlerCtx.Err(), context.Canceled)
					}
					release <- struct{}{}
					synctest.Wait()
					require.Len(t, result, 1)
					require.NoError(t, <-result)
					require.True(t, finished)
					require.Equal(t, 1, nextCalls)
				})
			})
		})
	}
}

type streamTestJetStream struct {
	jetstream.JetStream

	create func(context.Context, jetstream.StreamConfig) (jetstream.Stream, error)
}

func (js streamTestJetStream) CreateStream(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	return js.create(ctx, cfg)
}

func TestEnsureStream(t *testing.T) {
	t.Parallel()
	createErr := errors.New("create stream failed")
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "created"},
		{name: "already exists", err: jetstream.ErrStreamNameAlreadyInUse},
		{name: "wrapped already exists", err: fmt.Errorf("create: %w", jetstream.ErrStreamNameAlreadyInUse)},
		{name: "failure", err: createErr, want: createErr},
		{name: "canceled", err: context.Canceled, want: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			js := streamTestJetStream{
				create: func(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
					calls++
					require.Equal(t, t.Context(), ctx)
					require.Equal(t, jetstream.StreamConfig{
						Name: "users", Subjects: []string{"user.created", "user.updated"},
						Storage: jetstream.FileStorage, Duplicates: 15 * time.Minute,
					}, cfg)
					return nil, test.err
				},
			}
			err := ps_nats.EnsureStream(t.Context(), js, "users", "user.created", "user.updated")
			if test.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.want)
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestConnectInvalidURL(t *testing.T) {
	t.Parallel()
	connection, err := ps_nats.Connect("://invalid")
	require.Error(t, err)
	require.Nil(t, connection)
}
