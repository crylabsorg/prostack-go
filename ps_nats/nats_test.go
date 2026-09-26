package ps_nats

import (
	"context"
	"os"
	"testing"

	"github.com/nats-io/nats.go"
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
	js, err := client.JetStream()
	require.NoError(t, err)
	secondJS, err := client.JetStream()
	require.NoError(t, err)
	require.NotSame(t, js, secondJS)
	require.NoError(t, EnsureStream(js, stream, subject))
	other, err := Connect(url)
	require.NoError(t, err)
	otherJS, err := other.JetStream()
	require.NoError(t, err)
	require.NoError(t, EnsureStream(otherJS, stream, subject))
	other.Close()
	sub, err := js.PullSubscribe(subject, "", nats.BindStream(stream), nats.DeliverNew())
	require.NoError(t, err)
	defer sub.Unsubscribe()
	require.NoError(t, Publish(context.Background(), js, subject, nats.NewInbox(), []byte(`{"ok":true}`)))
	messages, err := sub.Fetch(1)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, []byte(`{"ok":true}`), messages[0].Data)
	require.NoError(t, messages[0].AckSync())
}
