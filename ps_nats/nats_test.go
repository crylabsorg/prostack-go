package ps_nats

import (
	"context"
	"os"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func TestJetStreamPublish(t *testing.T) {
	const stream = "PS_NATS_TEST"
	const subject = "ps_nats.test"
	url := os.Getenv("TEST_NATS_URL")
	if url == "" {
		t.Skip("set TEST_NATS_URL for NATS integration test")
	}
	client, err := Connect(url)
	require.NoError(t, err)
	defer client.Close()
	js, err := jetstream.New(client)
	require.NoError(t, err)
	secondJS, err := jetstream.New(client)
	require.NoError(t, err)
	require.NotSame(t, js, secondJS)
	require.NoError(t, EnsureStream(context.Background(), js, stream, subject))
	other, err := Connect(url)
	require.NoError(t, err)
	otherJS, err := jetstream.New(other)
	require.NoError(t, err)
	require.NoError(t, EnsureStream(context.Background(), otherJS, stream, subject))
	other.Close()
	consumer, err := js.CreateConsumer(context.Background(), stream, jetstream.ConsumerConfig{
		FilterSubject: subject, DeliverPolicy: jetstream.DeliverNewPolicy, AckPolicy: jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)
	require.NoError(t, Publish(context.Background(), js, subject, nats.NewInbox(), []byte(`{"ok":true}`)))
	batch, err := consumer.Fetch(1)
	require.NoError(t, err)
	var messages []jetstream.Msg
	for msg := range batch.Messages() {
		messages = append(messages, msg)
	}
	require.NoError(t, batch.Error())
	require.Len(t, messages, 1)
	require.Equal(t, []byte(`{"ok":true}`), messages[0].Data())
	require.NoError(t, messages[0].DoubleAck(context.Background()))
}
