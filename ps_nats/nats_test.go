package ps_nats_test

// import (
// 	"context"
// 	"errors"
// 	"testing"
// 	"testing/synctest"
// 	"time"

// 	"github.com/nats-io/nats.go"
// 	"github.com/nats-io/nats.go/jetstream"
// 	"github.com/stretchr/testify/require"
// )

// type consumeTestConsumer struct {
// 	jetstream.Consumer
// 	fetch func(int) (jetstream.MessageBatch, error)
// }

// func (c consumeTestConsumer) Fetch(size int, _ ...jetstream.FetchOpt) (jetstream.MessageBatch, error) {
// 	return c.fetch(size)
// }

// type consumeTestBatch struct {
// 	messages chan jetstream.Msg
// 	err      error
// }

// func (b consumeTestBatch) Messages() <-chan jetstream.Msg { return b.messages }
// func (b consumeTestBatch) Error() error                   { return b.err }

// type consumeTestMessage struct{ jetstream.Msg }

// func (consumeTestMessage) Headers() nats.Header { return nil }
// func (consumeTestMessage) Subject() string      { return "test" }

// func TestRun(t *testing.T) {
// 	t.Run("invalid parallelism", func(t *testing.T) {
// 		for _, size := range []int{0, -1} {
// 			require.Error(t, Run(context.Background(), context.Background(), nil, size, nil))
// 		}
// 	})
// 	t.Run("closed batch and workers", func(t *testing.T) {
// 		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
// 		defer cancel()
// 		batchErr := errors.New("batch failed")
// 		messages := make(chan jetstream.Msg, 2)
// 		messages <- consumeTestMessage{}
// 		messages <- consumeTestMessage{}
// 		close(messages)
// 		handled := make(chan struct{}, 2)
// 		consumer := consumeTestConsumer{fetch: func(size int) (jetstream.MessageBatch, error) {
// 			require.Equal(t, 2, size)
// 			return consumeTestBatch{messages: messages, err: batchErr}, nil
// 		}}
// 		err := Run(ctx, context.Background(), consumer, 2, func(context.Context, jetstream.Msg) error { handled <- struct{}{}; return nil })
// 		require.ErrorIs(t, err, batchErr)
// 		require.Len(t, handled, 2)
// 	})
// 	t.Run("cancel pending batch", func(t *testing.T) {
// 		ctx, cancel := context.WithCancel(context.Background())
// 		defer cancel()
// 		consumer := consumeTestConsumer{fetch: func(int) (jetstream.MessageBatch, error) {
// 			cancel()
// 			return consumeTestBatch{messages: make(chan jetstream.Msg)}, nil
// 		}}
// 		require.NoError(t, Run(ctx, context.Background(), consumer, 1, nil))
// 	})
// }

// func TestRunCancellation(t *testing.T) {
// 	for _, drain := range []bool{false, true} {
// 		name := "context"
// 		if drain {
// 			name = "drain"
// 		}
// 		t.Run(name, func(t *testing.T) {
// 			t.Run("before fetch", func(t *testing.T) {
// 				ctx, cancel := context.WithCancel(context.Background())
// 				cancel()
// 				workCtx, drainingCtx := ctx, context.Background()
// 				if drain {
// 					workCtx, drainingCtx = drainingCtx, workCtx
// 				}
// 				require.NoError(t, Run(workCtx, drainingCtx, nil, 1, nil))
// 			})
// 			t.Run("waiting for messages", func(t *testing.T) {
// 				synctest.Test(t, func(t *testing.T) {
// 					ctx, cancel := context.WithCancel(context.Background())
// 					defer cancel()
// 					workCtx, drainingCtx := ctx, context.Background()
// 					if drain {
// 						workCtx, drainingCtx = drainingCtx, workCtx
// 					}
// 					fetches := 0
// 					consumer := consumeTestConsumer{fetch: func(int) (jetstream.MessageBatch, error) {
// 						fetches++
// 						return consumeTestBatch{messages: make(chan jetstream.Msg)}, nil
// 					}}
// 					result := make(chan error, 1)
// 					go func() {
// 						result <- Run(workCtx, drainingCtx, consumer, 1, nil)
// 					}()
// 					synctest.Wait()
// 					require.Equal(t, 1, fetches)
// 					require.Empty(t, result)
// 					cancel()
// 					synctest.Wait()
// 					require.Len(t, result, 1)
// 					require.NoError(t, <-result)
// 					require.Equal(t, 1, fetches)
// 				})
// 			})
// 			for _, closeBatch := range []bool{false, true} {
// 				name := "active worker and pending batch"
// 				if closeBatch {
// 					name = "waiting for worker slot"
// 				}
// 				t.Run(name, func(t *testing.T) {
// 					synctest.Test(t, func(t *testing.T) {
// 						ctx, cancel := context.WithCancel(context.Background())
// 						defer cancel()
// 						workCtx, drainingCtx := ctx, context.Background()
// 						if drain {
// 							workCtx, drainingCtx = drainingCtx, workCtx
// 						}
// 						messages := make(chan jetstream.Msg, 1)
// 						messages <- consumeTestMessage{}
// 						if closeBatch {
// 							close(messages)
// 						}
// 						fetches := 0
// 						consumer := consumeTestConsumer{fetch: func(int) (jetstream.MessageBatch, error) {
// 							fetches++
// 							return consumeTestBatch{messages: messages}, nil
// 						}}
// 						release := make(chan struct{})
// 						defer close(release)
// 						var handlerCtx context.Context
// 						finished := false
// 						result := make(chan error, 1)
// 						go func() {
// 							result <- Run(workCtx, drainingCtx, consumer, 1, func(ctx context.Context, _ jetstream.Msg) error {
// 								handlerCtx = ctx
// 								<-release
// 								finished = true
// 								return nil
// 							})
// 						}()
// 						synctest.Wait()
// 						require.NotNil(t, handlerCtx)
// 						cancel()
// 						synctest.Wait()
// 						require.Empty(t, result, "Run must wait for the active worker")
// 						require.False(t, finished)
// 						if drain {
// 							require.NoError(t, handlerCtx.Err(), "draining must not cancel the worker context")
// 						} else {
// 							require.ErrorIs(t, handlerCtx.Err(), context.Canceled)
// 						}
// 						if !closeBatch {
// 							messages <- consumeTestMessage{}
// 						}
// 						release <- struct{}{}
// 						synctest.Wait()
// 						require.Len(t, result, 1)
// 						require.NoError(t, <-result)
// 						require.True(t, finished)
// 						require.Equal(t, 1, fetches)
// 						if !closeBatch {
// 							require.Len(t, messages, 1, "draining must leave new messages unhandled")
// 						}
// 					})
// 				})
// 			}
// 		})
// 	}
// }
