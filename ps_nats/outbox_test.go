package ps_nats

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type testOutboxMessage struct{ id string }

func (m testOutboxMessage) ID() string      { return m.id }
func (m testOutboxMessage) Subject() string { return "test.created" }
func (m testOutboxMessage) Payload() []byte { return []byte(`{"created":true}`) }

type testOutbox struct {
	messages    []OutboxMessage
	results     []OutboxMessagePublishResult
	completeErr error
}

func (o *testOutbox) ProcessOutboxBatch(ctx context.Context, publish func([]OutboxMessage) []OutboxMessagePublishResult) error {
	if len(o.messages) > 0 {
		o.results = publish(o.messages)
	}
	return o.completeErr
}

type testJetStream struct {
	jetstream.JetStream
	publish func(context.Context, *nats.Msg) (*jetstream.PubAck, error)
}

func (js *testJetStream) PublishMsg(ctx context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	return js.publish(ctx, msg)
}

func TestPublishBatchPartialSuccess(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	restore := zap.ReplaceGlobals(zap.New(core))
	defer restore()
	publishErr := errors.New("NATS rejected message")
	completeErr := errors.New("commit failed")
	outbox := &testOutbox{messages: []OutboxMessage{testOutboxMessage{"success"}, testOutboxMessage{"failure"}}, completeErr: completeErr}
	js := &testJetStream{publish: func(ctx context.Context, msg *nats.Msg) (*jetstream.PubAck, error) {
		if msg.Header.Get(nats.MsgIdHdr) == "failure" {
			return nil, publishErr
		}
		return &jetstream.PubAck{}, nil
	}}
	err := (&outboxPublisher{js: js, outbox: outbox}).publishBatch(context.Background())
	results := outbox.results
	require.ErrorIs(t, err, completeErr)
	require.NotErrorIs(t, err, publishErr)
	require.Len(t, results, 2)
	require.Equal(t, outbox.messages[0], results[0].OutboxMessage)
	require.NoError(t, results[0].Err)
	require.Equal(t, outbox.messages[1], results[1].OutboxMessage)
	require.ErrorIs(t, results[1].Err, publishErr)
	require.Equal(t, 1, logs.Len())
	require.Equal(t, "failure", logs.All()[0].ContextMap()["id"])
	require.Contains(t, logs.All()[0].ContextMap()["error"], publishErr.Error())
}

func TestPublishBatchCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outbox := &testOutbox{messages: []OutboxMessage{testOutboxMessage{"canceled"}}}
	err := (&outboxPublisher{outbox: outbox}).publishBatch(ctx)
	results := outbox.results
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, outbox.messages[0], results[0].OutboxMessage)
	require.ErrorIs(t, results[0].Err, context.Canceled)
}

func TestPublisherWaitsForInFlightBatchOnDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	drainingCtx, drain := context.WithCancel(context.Background())
	defer drain()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() { releaseOnce.Do(func() { close(release) }) }
	defer closeRelease()
	outbox := &testOutbox{messages: []OutboxMessage{testOutboxMessage{"in-flight"}}}
	js := &testJetStream{publish: func(ctx context.Context, msg *nats.Msg) (*jetstream.PubAck, error) {
		close(started)
		select {
		case <-release:
			return &jetstream.PubAck{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	publisher := outboxPublisher{js: js, outbox: outbox, ticker: time.NewTicker(time.Second)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		publisher.run(ctx, drainingCtx)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("publisher did not start")
	}
	drain()
	select {
	case <-done:
		t.Fatal("publisher stopped before finishing its batch")
	default:
	}
	closeRelease()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("publisher did not stop")
	}
}
