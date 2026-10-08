package ps_nats

// import (
// 	"context"
// 	"os"
// 	"testing"

// 	"github.com/nats-io/nats.go"
// 	"github.com/nats-io/nats.go/jetstream"
// 	"github.com/stretchr/testify/require"
// )

// func TestJetStreamPublish(t *testing.T) {
// 	const stream = "PS_NATS_TEST"
// 	const subject = "ps_nats.test"
// 	url := os.Getenv("TEST_NATS_URL")
// 	if url == "" {
// 		t.Skip("set TEST_NATS_URL for NATS integration test")
// 	}
// 	client, err := Connect(url)
// 	require.NoError(t, err)
// 	defer client.Close()
// 	js, err := jetstream.New(client)
// 	require.NoError(t, err)
// 	secondJS, err := jetstream.New(client)
// 	require.NoError(t, err)
// 	require.NotSame(t, js, secondJS)
// 	require.NoError(t, EnsureStream(context.Background(), js, stream, subject))
// 	other, err := Connect(url)
// 	require.NoError(t, err)
// 	otherJS, err := jetstream.New(other)
// 	require.NoError(t, err)
// 	require.NoError(t, EnsureStream(context.Background(), otherJS, stream, subject))
// 	other.Close()
// 	consumer, err := js.CreateConsumer(context.Background(), stream, jetstream.ConsumerConfig{
// 		FilterSubject: subject, DeliverPolicy: jetstream.DeliverNewPolicy, AckPolicy: jetstream.AckExplicitPolicy,
// 	})
// 	require.NoError(t, err)
// 	messagesToPublish := []OutboxMessage{
// 		testOutboxMessage{subject: subject, id: nats.NewInbox(), payload: []byte(`{"id":1}`)},
// 		testOutboxMessage{subject: subject, id: nats.NewInbox(), payload: []byte(`{"id":2}`)},
// 	}
// 	publisher := &OutboxPublisher{JS: js}
// 	for _, err := range publisher.publishMessages(context.Background(), messagesToPublish) {
// 		require.NoError(t, err)
// 	}
// 	batch, err := consumer.Fetch(2)
// 	require.NoError(t, err)
// 	var messages []jetstream.Msg
// 	for msg := range batch.Messages() {
// 		messages = append(messages, msg)
// 	}
// 	require.NoError(t, batch.Error())
// 	require.Len(t, messages, 2)
// 	require.ElementsMatch(t, [][]byte{[]byte(`{"id":1}`), []byte(`{"id":2}`)}, [][]byte{messages[0].Data(), messages[1].Data()})
// 	for _, msg := range messages {
// 		require.NoError(t, msg.DoubleAck(context.Background()))
// 	}
// }

// type testOutboxMessage struct {
// 	subject string
// 	id      string
// 	payload []byte
// }

// func (m testOutboxMessage) Subject() string { return m.subject }
// func (m testOutboxMessage) ID() string      { return m.id }
// func (m testOutboxMessage) Payload() []byte { return m.payload }

// type testOutbox struct {
// 	nextCtx             context.Context
// 	completeCtx         context.Context
// 	rollbackErr         error
// 	rollbackHasDeadline bool
// }

// func (o *testOutbox) NextBatch(ctx context.Context, _ int) ([]OutboxMessage, error) {
// 	o.nextCtx = ctx
// 	return []OutboxMessage{testOutboxMessage{subject: "test.subject", id: "test-id"}}, nil
// }

// func (o *testOutbox) CompleteBatch(ctx context.Context, _ []OutboxMessage) error {
// 	o.completeCtx = ctx
// 	return nil
// }

// func (o *testOutbox) RollbackBatch(ctx context.Context, _ []OutboxMessage) error {
// 	o.rollbackErr = ctx.Err()
// 	_, o.rollbackHasDeadline = ctx.Deadline()
// 	return nil
// }

// type testJetStream struct {
// 	jetstream.JetStream
// 	publishCtx context.Context
// 	cancel     context.CancelFunc
// }

// func (js *testJetStream) PublishMsg(ctx context.Context, _ *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
// 	js.publishCtx = ctx
// 	if js.cancel != nil {
// 		js.cancel()
// 		return nil, context.Canceled
// 	}
// 	return &jetstream.PubAck{}, nil
// }

// func TestOutboxPublisherContexts(t *testing.T) {
// 	outbox := &testOutbox{}
// 	js := &testJetStream{}
// 	completed, err := (&OutboxPublisher{JS: js, Outbox: outbox}).publishBatch(context.Background(), 1)
// 	require.NoError(t, err)
// 	require.Equal(t, 1, completed)
// 	_, hasDeadline := outbox.nextCtx.Deadline()
// 	require.True(t, hasDeadline)
// 	_, hasDeadline = js.publishCtx.Deadline()
// 	require.False(t, hasDeadline)
// 	_, hasDeadline = outbox.completeCtx.Deadline()
// 	require.False(t, hasDeadline)

// 	ctx, cancel := context.WithCancel(context.Background())
// 	outbox = &testOutbox{}
// 	js = &testJetStream{cancel: cancel}
// 	completed, err = (&OutboxPublisher{JS: js, Outbox: outbox}).publishBatch(ctx, 1)
// 	require.ErrorIs(t, err, context.Canceled)
// 	require.Zero(t, completed)
// 	require.NoError(t, outbox.rollbackErr)
// 	require.True(t, outbox.rollbackHasDeadline)
// }
